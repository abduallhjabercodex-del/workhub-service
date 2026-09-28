package handler

import (
	"embed"
	"net/http"
)

//go:embed assets/*
var swaggerAssets embed.FS

func ServeSwaggerAssets() http.Handler {
	return http.FileServer(http.FS(swaggerAssets))
}
