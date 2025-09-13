package ctx

import (
	"context"
)

type Context interface {
	AbortWithJSON(code int, jsonObj interface{})
	Set(key string, value interface{})
	Next(c context.Context)
	GetHeader(key string) []byte
	ClientIP() string
	Value(key interface{}) interface{}
}
