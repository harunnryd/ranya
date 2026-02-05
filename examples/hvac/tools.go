package main

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/harunnryd/ranya/pkg/llm"
)

type HVACToolRegistry struct {
	tools    []llm.Tool
	handlers map[string]func(map[string]any) (string, error)
}

func NewHVACToolRegistry() *HVACToolRegistry {
	reg := &HVACToolRegistry{}
	reg.tools = []llm.Tool{
		{
			Name:        "estimate_service_cost",
			Description: "Estimasi biaya servis AC berdasarkan jenis perangkat, issue, dan urgensi.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"equipment_type": map[string]any{"type": "string"},
					"issue_summary":  map[string]any{"type": "string"},
					"urgency":        map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
				},
				"required": []string{"equipment_type", "issue_summary", "urgency"},
			},
		},
		{
			Name:                 "schedule_visit",
			Description:          "Jadwalkan kunjungan teknisi sesuai lokasi dan waktu yang diinginkan.",
			RequiresConfirmation: true,
			ConfirmationPromptByLanguage: map[string]string{
				"id": "Sebelum saya jadwalkan kunjungan, apakah Anda ingin saya lanjutkan?",
				"en": "Before I schedule the visit, do you want me to proceed?",
			},
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location":       map[string]any{"type": "string"},
					"preferred_time": map[string]any{"type": "string"},
					"urgency":        map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
				},
				"required": []string{"location", "preferred_time", "urgency"},
			},
		},
		{
			Name:                 "create_ticket",
			Description:          "Buat tiket layanan HVAC untuk follow up.",
			RequiresConfirmation: true,
			ConfirmationPromptByLanguage: map[string]string{
				"id": "Sebelum saya buat tiket, boleh saya lanjutkan?",
				"en": "Before I create the ticket, may I proceed?",
			},
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"customer_id":   map[string]any{"type": "string"},
					"issue_summary": map[string]any{"type": "string"},
					"location":      map[string]any{"type": "string"},
				},
				"required": []string{"customer_id", "issue_summary", "location"},
			},
		},
		{
			Name:                 "send_payment_link",
			Description:          "Kirim tautan pembayaran untuk biaya servis.",
			RequiresConfirmation: true,
			ConfirmationPromptByLanguage: map[string]string{
				"id": "Sebelum saya kirim link pembayaran, apakah Anda setuju?",
				"en": "Before I send the payment link, do you agree?",
			},
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"customer_id": map[string]any{"type": "string"},
					"amount":      map[string]any{"type": "string"},
				},
				"required": []string{"customer_id", "amount"},
			},
		},
		{
			Name:        "check_hvac_inventory",
			Description: "Cek stok part atau item HVAC.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"item_name": map[string]any{"type": "string"},
				},
				"required": []string{"item_name"},
			},
		},
		{
			Name:        "get_technician_eta",
			Description: "Perkiraan kedatangan teknisi berdasarkan lokasi dan urgensi.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{"type": "string"},
					"urgency":  map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
				},
				"required": []string{"location", "urgency"},
			},
		},
	}
	reg.handlers = map[string]func(map[string]any) (string, error){
		"estimate_service_cost": estimateServiceCostTool,
		"schedule_visit":        scheduleVisitTool,
		"create_ticket":         createTicketTool,
		"send_payment_link":     sendPaymentLinkTool,
		"check_hvac_inventory":  checkHVACInventoryTool,
		"get_technician_eta":    getTechnicianETATool,
	}
	return reg
}

func (r *HVACToolRegistry) Tools() []llm.Tool {
	return r.tools
}

func (r *HVACToolRegistry) HandleTool(name string, args map[string]any) (string, error) {
	h := r.handlers[name]
	if h == nil {
		return "", errors.New("missing handler")
	}
	return h(args)
}

var _ llm.ToolRegistry = (*HVACToolRegistry)(nil)

func estimateServiceCostTool(args map[string]any) (string, error) {
	equipment, err := requiredString(args, "equipment_type")
	if err != nil {
		return "", err
	}
	issue, err := requiredString(args, "issue_summary")
	if err != nil {
		return "", err
	}
	urgency, err := requiredString(args, "urgency")
	if err != nil {
		return "", err
	}

	base := 250_000
	lowerEq := strings.ToLower(equipment)
	switch {
	case strings.Contains(lowerEq, "central"):
		base = 650_000
	case strings.Contains(lowerEq, "inverter"):
		base = 400_000
	case strings.Contains(lowerEq, "cassette"):
		base = 500_000
	case strings.Contains(lowerEq, "standing"):
		base = 450_000
	}

	urgencyAdj := 0
	switch strings.ToLower(urgency) {
	case "high":
		urgencyAdj = 120_000
	case "medium":
		urgencyAdj = 60_000
	}

	seed := equipment + "|" + issue + "|" + urgency
	variation := (stableInt(seed, 11) - 5) * 10_000 // -50k .. +50k
	estimate := base + urgencyAdj + variation
	if estimate < 150_000 {
		estimate = 150_000
	}
	min := estimate - 50_000
	max := estimate + 50_000

	return fmt.Sprintf("Estimasi biaya servis untuk %s (issue: %s) sekitar Rp%d–Rp%d, belum termasuk spare part.", equipment, issue, min, max), nil
}

func scheduleVisitTool(args map[string]any) (string, error) {
	location, err := requiredString(args, "location")
	if err != nil {
		return "", err
	}
	preferred, err := requiredString(args, "preferred_time")
	if err != nil {
		return "", err
	}
	urgency, err := requiredString(args, "urgency")
	if err != nil {
		return "", err
	}

	slots := []string{
		"Hari ini 14:00-16:00",
		"Besok 09:00-11:00",
		"Besok 13:00-15:00",
		"Lusa 10:00-12:00",
	}
	seed := location + "|" + preferred + "|" + urgency
	slot := slots[stableInt(seed, len(slots))]
	bookingID := "HVAC-" + strconv.Itoa(100000+stableInt(seed, 899999))
	return fmt.Sprintf("Slot %s tersedia untuk %s. Preferensi: %s. Kode booking: %s.", slot, location, preferred, bookingID), nil
}

func createTicketTool(args map[string]any) (string, error) {
	customerID, err := requiredString(args, "customer_id")
	if err != nil {
		return "", err
	}
	issue, err := requiredString(args, "issue_summary")
	if err != nil {
		return "", err
	}
	location, err := requiredString(args, "location")
	if err != nil {
		return "", err
	}
	seed := customerID + "|" + issue + "|" + location
	ticketID := "TKT-" + strconv.Itoa(10000+stableInt(seed, 89999))
	return fmt.Sprintf("Tiket %s dibuat untuk %s di %s. Issue: %s.", ticketID, customerID, location, issue), nil
}

func sendPaymentLinkTool(args map[string]any) (string, error) {
	customerID, err := requiredString(args, "customer_id")
	if err != nil {
		return "", err
	}
	amount, err := requiredString(args, "amount")
	if err != nil {
		return "", err
	}
	seed := customerID + "|" + amount
	token := stableInt(seed, 999999)
	link := fmt.Sprintf("https://pay.example/hvac/%06d", token)
	return fmt.Sprintf("Link pembayaran untuk %s sebesar %s: %s", customerID, amount, link), nil
}

func checkHVACInventoryTool(args map[string]any) (string, error) {
	item, err := requiredString(args, "item_name")
	if err != nil {
		return "", err
	}
	seed := strings.ToLower(item)
	qty := 1 + stableInt(seed, 12)
	location := "Gudang Bekasi"
	if stableInt(seed, 2) == 1 {
		location = "Gudang Tangerang"
	}
	return fmt.Sprintf("Stok %s tersedia %d unit di %s.", item, qty, location), nil
}

func getTechnicianETATool(args map[string]any) (string, error) {
	location, err := requiredString(args, "location")
	if err != nil {
		return "", err
	}
	urgency, err := requiredString(args, "urgency")
	if err != nil {
		return "", err
	}
	seed := location + "|" + urgency
	min := 25 + stableInt(seed, 20)
	max := min + 15
	return fmt.Sprintf("Perkiraan teknisi tiba %d-%d menit untuk area %s.", min, max, location), nil
}

func requiredString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("invalid %s", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("missing %s", key)
	}
	return s, nil
}

func stableInt(seed string, mod int) int {
	if mod <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return int(h.Sum32() % uint32(mod))
}
