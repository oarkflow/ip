package ctx

type Context interface {
	Set(key string, value any)
	Next() error
	Get(key string, def ...string) string
	IP() string
	Locals(key any, value ...any) any
	JSON(data any, ctype ...string) error
	Status(code int) Context
}
