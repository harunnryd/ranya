package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/harunnryd/ranya/pkg/configutil"
	"github.com/harunnryd/ranya/pkg/transports"
	twiliotransport "github.com/harunnryd/ranya/pkg/transports/twilio"
	"github.com/spf13/viper"
)

type twilioConfig struct {
	Transports struct {
		Provider string         `mapstructure:"provider"`
		Settings map[string]any `mapstructure:"settings"`
	} `mapstructure:"transports"`
}

type twilioSettings struct {
	AccountSID string `mapstructure:"account_sid"`
	AuthToken  string `mapstructure:"auth_token"`
	PublicURL  string `mapstructure:"public_url"`
	VoicePath  string `mapstructure:"voice_path"`
}

func main() {
	configPath := flag.String("config", "examples/hvac/config.local.yaml", "")
	from := flag.String("from", "", "")
	to := flag.String("to", "", "")
	voiceURL := flag.String("voice_url", "", "")
	sendDigits := flag.String("send_digits", "", "")
	flag.Parse()
	if *from == "" || *to == "" {
		fmt.Println("usage: make_call -from=+123 -to=+456 [-config=...]")
		os.Exit(1)
	}
	cfg, err := loadTwilioConfig(*configPath)
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	var settings twilioSettings
	if err := configutil.DecodeSettings(cfg.Transports.Settings, &settings); err != nil {
		fmt.Println("settings error:", err)
		os.Exit(1)
	}
	url := *voiceURL
	if url == "" {
		if settings.PublicURL == "" {
			fmt.Println("public_url is empty")
			os.Exit(1)
		}
		voicePath := settings.VoicePath
		if voicePath == "" {
			voicePath = "/voice"
		}
		url = "https://" + normalizePublicURL(settings.PublicURL) + voicePath
	}
	dialer := twiliotransport.NewDialer(twiliotransport.Config{
		AccountSID: settings.AccountSID,
		AuthToken:  settings.AuthToken,
		PublicURL:  settings.PublicURL,
		VoicePath:  settings.VoicePath,
	})
	if *sendDigits != "" {
		callSID, err := dialer.DialWithOptions(context.Background(), *to, *from, url, transports.DialOptions{SendDigits: *sendDigits})
		if err != nil {
			fmt.Println("call error:", err)
			os.Exit(1)
		}
		fmt.Println("call_sid:", callSID)
		return
	}
	callSID, err := dialer.Dial(context.Background(), *to, *from, url)
	if err != nil {
		fmt.Println("call error:", err)
		os.Exit(1)
	}
	fmt.Println("call_sid:", callSID)
}

func loadTwilioConfig(path string) (twilioConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return twilioConfig{}, err
	}
	var cfg twilioConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return twilioConfig{}, err
	}
	return cfg, nil
}

func normalizePublicURL(v string) string {
	if v == "" {
		return ""
	}
	if len(v) >= 8 && v[:8] == "https://" {
		return v[8:]
	}
	if len(v) >= 7 && v[:7] == "http://" {
		return v[7:]
	}
	for len(v) > 0 && v[len(v)-1] == '/' {
		v = v[:len(v)-1]
	}
	return v
}
