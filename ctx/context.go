package ctx

type Context interface {
	Set(key string, value string)
	Next() error
	Get(key string, def ...string) string
	IP() string
	Locals(key any, value ...any) any
	JSON(data any, ctype ...string) error
}
