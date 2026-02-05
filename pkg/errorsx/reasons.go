package errorsx

// ReasonCode is a short machine-readable error reason.
type ReasonCode string

const (
	ReasonUnknown ReasonCode = "unknown"

	ReasonSTTConnect     ReasonCode = "stt_connect"
	ReasonSTTSend        ReasonCode = "stt_send"
	ReasonSTTRetry       ReasonCode = "stt_retry"
	ReasonSTTRateLimit   ReasonCode = "stt_rate_limit"
	ReasonSTTCircuitOpen ReasonCode = "stt_circuit_open"

	ReasonTTSConnect     ReasonCode = "tts_connect"
	ReasonTTSSend        ReasonCode = "tts_send"
	ReasonTTSRetry       ReasonCode = "tts_retry"
	ReasonTTSRateLimit   ReasonCode = "tts_rate_limit"
	ReasonTTSCircuitOpen ReasonCode = "tts_circuit_open"

	ReasonLLMGenerate  ReasonCode = "llm_generate"
	ReasonLLMStream    ReasonCode = "llm_stream"
	ReasonLLMRateLimit ReasonCode = "llm_rate_limit"

	ReasonTransportInvalidSignature ReasonCode = "webhook_invalid_signature"
	ReasonTransportSend             ReasonCode = "transport_send"
)
