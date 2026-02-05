package frames

import (
	"sync"
	"time"
)

type Kind string

const (
	KindAudio   Kind = "audio"
	KindText    Kind = "text"
	KindControl Kind = "control"
	KindSystem  Kind = "system"
	KindImage   Kind = "image"
)

type ControlCode string

const (
	ControlCancel            ControlCode = "cancel"
	ControlFlush             ControlCode = "flush"
	ControlStartInterruption ControlCode = "start_interruption"
	ControlFallback          ControlCode = "fallback"
	ControlHandoff           ControlCode = "handoff"
	ControlToolCall          ControlCode = "tool_call"
	ControlAudioReady        ControlCode = "audio_ready"
	ControlDTMF              ControlCode = "dtmf"
)

type Frame interface {
	Kind() Kind
	PTS() int64
	Meta() map[string]string
}

type AudioFrame struct {
	pts    int64
	data   []byte
	rate   int
	ch     int
	meta   map[string]string
	pooled bool
}

func NewAudioFrame(streamID string, pts int64, data []byte, rate, ch int, meta map[string]string) AudioFrame {
	return AudioFrame{
		pts:  pts,
		data: data,
		rate: rate,
		ch:   ch,
		meta: mergeMeta(streamID, meta),
	}
}

func NewAudioFrameFromPool(streamID string, pts int64, data []byte, rate, ch int, meta map[string]string) AudioFrame {
	buf := AcquireAudioBuf(len(data))
	copy(buf, data)
	return AudioFrame{
		pts:    pts,
		data:   buf,
		rate:   rate,
		ch:     ch,
		meta:   mergeMeta(streamID, meta),
		pooled: true,
	}
}

func (a AudioFrame) Kind() Kind              { return KindAudio }
func (a AudioFrame) PTS() int64              { return a.pts }
func (a AudioFrame) Meta() map[string]string { return cloneMeta(a.meta) }
func (a AudioFrame) Data() []byte            { return append([]byte(nil), a.data...) }
func (a AudioFrame) RawPayload() []byte      { return a.data }
func (a AudioFrame) Rate() int               { return a.rate }
func (a AudioFrame) Channels() int           { return a.ch }

func ReleaseAudioFrame(f Frame) bool {
	af, ok := f.(AudioFrame)
	if !ok {
		if ap, ok := f.(*AudioFrame); ok {
			af = *ap
		} else {
			return false
		}
	}
	if af.pooled {
		ReleaseAudioBuf(af.data)
		return true
	}
	return false
}

type TextFrame struct {
	pts  int64
	text string
	meta map[string]string
}

func NewTextFrame(streamID string, pts int64, text string, meta map[string]string) TextFrame {
	return TextFrame{
		pts:  pts,
		text: text,
		meta: mergeMeta(streamID, meta),
	}
}

func (t TextFrame) Kind() Kind              { return KindText }
func (t TextFrame) PTS() int64              { return t.pts }
func (t TextFrame) Meta() map[string]string { return cloneMeta(t.meta) }
func (t TextFrame) Text() string            { return t.text }

type ControlFrame struct {
	pts  int64
	code ControlCode
	meta map[string]string
}

func NewControlFrame(streamID string, pts int64, code ControlCode, meta map[string]string) ControlFrame {
	return ControlFrame{
		pts:  pts,
		code: code,
		meta: mergeMeta(streamID, meta),
	}
}

func (c ControlFrame) Kind() Kind              { return KindControl }
func (c ControlFrame) PTS() int64              { return c.pts }
func (c ControlFrame) Meta() map[string]string { return cloneMeta(c.meta) }
func (c ControlFrame) Code() ControlCode       { return c.code }

type SystemFrame struct {
	pts  int64
	name string
	meta map[string]string
}

func NewSystemFrame(streamID string, pts int64, name string, meta map[string]string) SystemFrame {
	return SystemFrame{
		pts:  pts,
		name: name,
		meta: mergeMeta(streamID, meta),
	}
}

func (s SystemFrame) Kind() Kind              { return KindSystem }
func (s SystemFrame) PTS() int64              { return s.pts }
func (s SystemFrame) Meta() map[string]string { return cloneMeta(s.meta) }
func (s SystemFrame) Name() string            { return s.name }

type ImageFrame struct {
	pts    int64
	data   []byte
	mime   string
	url    string
	meta   map[string]string
	pooled bool
}

func NewImageFrame(streamID string, pts int64, data []byte, mime, url string, meta map[string]string) ImageFrame {
	return ImageFrame{
		pts:  pts,
		data: data,
		mime: mime,
		url:  url,
		meta: mergeMeta(streamID, meta),
	}
}

func NewImageFrameFromPool(streamID string, pts int64, data []byte, mime, url string, meta map[string]string) ImageFrame {
	buf := AcquireImageBuf(len(data))
	copy(buf, data)
	return ImageFrame{
		pts:    pts,
		data:   buf,
		mime:   mime,
		url:    url,
		meta:   mergeMeta(streamID, meta),
		pooled: true,
	}
}

func (i ImageFrame) Kind() Kind              { return KindImage }
func (i ImageFrame) PTS() int64              { return i.pts }
func (i ImageFrame) Meta() map[string]string { return cloneMeta(i.meta) }
func (i ImageFrame) Data() []byte            { return append([]byte(nil), i.data...) }
func (i ImageFrame) RawPayload() []byte      { return i.data }
func (i ImageFrame) MIME() string            { return i.mime }
func (i ImageFrame) URL() string             { return i.url }

func ReleaseImageFrame(f Frame) bool {
	im, ok := f.(ImageFrame)
	if !ok {
		if ip, ok := f.(*ImageFrame); ok {
			im = *ip
		} else {
			return false
		}
	}
	if im.pooled {
		ReleaseImageBuf(im.data)
		return true
	}
	return false
}

type PTSGen struct {
	mu    sync.Mutex
	value map[string]int64
}

func NewPTSGen() *PTSGen {
	return &PTSGen{value: make(map[string]int64)}
}

func (g *PTSGen) Next(streamID string) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	v := g.value[streamID] + time.Millisecond.Nanoseconds()
	g.value[streamID] = v
	return v
}

var audioBufPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 4096)
	},
}

func AcquireAudioBuf(size int) []byte {
	b := audioBufPool.Get().([]byte)
	if cap(b) < size {
		return make([]byte, size)
	}
	return b[:size]
}

func ReleaseAudioBuf(b []byte) {
	audioBufPool.Put(b[:0])
}

var imageBufPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 8192)
	},
}

func AcquireImageBuf(size int) []byte {
	b := imageBufPool.Get().([]byte)
	if cap(b) < size {
		return make([]byte, size)
	}
	return b[:size]
}

func ReleaseImageBuf(b []byte) {
	imageBufPool.Put(b[:0])
}

func mergeMeta(streamID string, meta map[string]string) map[string]string {
	out := make(map[string]string, 2+len(meta))
	if streamID != "" {
		out[MetaStreamID] = streamID
	}
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func cloneMeta(meta map[string]string) map[string]string {
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}
