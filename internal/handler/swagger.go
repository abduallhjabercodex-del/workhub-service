package handler

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

// swaggerUI returns the Swagger UI HTML page (uses CDN assets, no extra deps needed).
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>WorkHub API — Swagger UI</title>
  <link rel="icon" type="image/svg+xml"
        href="https://unpkg.com/swagger-ui-dist@5/favicon-32x32.png" />
  <link rel="stylesheet"
        href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <style>
    /* ── brand overrides ── */
    :root {
      --primary:   #6c47ff;
      --primary-dk:#4f2fe0;
      --bg:        #0d0f1a;
      --surface:   #161827;
      --border:    #2a2d44;
      --text:      #e2e4f0;
      --muted:     #9496b0;
    }
    * { box-sizing: border-box; }

    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font-family: 'Inter', 'Segoe UI', system-ui, sans-serif;
    }

    /* top nav bar */
    #topbar {
      display: flex;
      align-items: center;
      gap: 14px;
      padding: 0 28px;
      height: 58px;
      background: linear-gradient(90deg, var(--primary-dk) 0%, var(--primary) 100%);
      box-shadow: 0 2px 16px rgba(108,71,255,.45);
    }
    #topbar svg { flex-shrink: 0; }
    #topbar h1 {
      margin: 0;
      font-size: 1.05rem;
      font-weight: 700;
      color: #fff;
      letter-spacing: .02em;
    }
    #topbar span {
      margin-left: auto;
      font-size: .78rem;
      color: rgba(255,255,255,.65);
      font-weight: 500;
    }

    /* swagger-ui wrappers */
    #swagger-ui { max-width: 1080px; margin: 0 auto; padding: 28px 16px 60px; }

    .swagger-ui .topbar          { display: none; }
    .swagger-ui .info            { margin-bottom: 28px; }
    .swagger-ui .info .title     { color: var(--text) !important; font-size: 1.8rem; }
    .swagger-ui .info .description p { color: var(--muted); }

    .swagger-ui .scheme-container {
      background: var(--surface);
      box-shadow: none;
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 12px 20px;
      margin-bottom: 20px;
    }

    /* operation blocks */
    .swagger-ui .opblock {
      border-radius: 10px !important;
      margin-bottom: 10px;
      border: 1px solid var(--border) !important;
      background: var(--surface) !important;
      box-shadow: 0 2px 8px rgba(0,0,0,.25);
      overflow: hidden;
    }
    .swagger-ui .opblock .opblock-summary {
      border-radius: 0 !important;
      border-bottom: none;
    }
    .swagger-ui .opblock-summary-path,
    .swagger-ui .opblock-summary-path__deprecated { color: var(--text) !important; }
    .swagger-ui .opblock-summary-description       { color: var(--muted) !important; }

    /* method badges */
    .swagger-ui .opblock.opblock-get    .opblock-summary-method { background: #0e8a6a; }
    .swagger-ui .opblock.opblock-post   .opblock-summary-method { background: #3670d8; }
    .swagger-ui .opblock.opblock-patch  .opblock-summary-method { background: #c47a00; }
    .swagger-ui .opblock.opblock-delete .opblock-summary-method { background: #c43040; }
    .swagger-ui .opblock-summary-method {
      border-radius: 6px !important;
      font-weight: 700 !important;
      min-width: 72px;
    }

    /* section titles */
    .swagger-ui .opblock-tag {
      color: var(--text) !important;
      border-bottom: 1px solid var(--border) !important;
      font-size: 1.1rem !important;
      font-weight: 600 !important;
    }

    /* "Try it out" + Execute buttons */
    .swagger-ui .btn.try-out__btn,
    .swagger-ui .btn.execute          { border-radius: 6px !important; }
    .swagger-ui .btn.execute {
      background: var(--primary) !important;
      border-color: var(--primary) !important;
      color: #fff !important;
    }
    .swagger-ui .btn.execute:hover { background: var(--primary-dk) !important; }

    /* response codes */
    .swagger-ui .response-col_status { color: var(--text) !important; }
    .swagger-ui table thead tr th    { color: var(--muted) !important; border-bottom: 1px solid var(--border); }

    /* inputs */
    .swagger-ui input[type=text],
    .swagger-ui select,
    .swagger-ui textarea {
      background: var(--bg) !important;
      border: 1px solid var(--border) !important;
      color: var(--text) !important;
      border-radius: 6px !important;
    }

    /* scrollbar */
    ::-webkit-scrollbar { width: 6px; height: 6px; }
    ::-webkit-scrollbar-track { background: var(--bg); }
    ::-webkit-scrollbar-thumb { background: var(--border); border-radius: 3px; }
  </style>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet" />
</head>
<body>

<!-- branded top bar -->
<nav id="topbar">
  <svg width="28" height="28" viewBox="0 0 32 32" fill="none">
    <rect width="32" height="32" rx="8" fill="rgba(255,255,255,.15)"/>
    <path d="M8 22l6-12 4 8 3-5 5 9H8z" fill="#fff"/>
  </svg>
  <h1>WorkHub API</h1>
  <span>Interactive Documentation</span>
</nav>

<div id="swagger-ui"></div>

<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
<script>
  window.onload = () => {
    SwaggerUIBundle({
      url:          "/api/v1/docs/openapi.yaml",
      dom_id:       "#swagger-ui",
      presets:      [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
      layout:       "StandaloneLayout",
      deepLinking:  true,
      displayRequestDuration: true,
      tryItOutEnabled: true,
      defaultModelsExpandDepth: 1,
      defaultModelExpandDepth:  2,
    });
  };
</script>
</body>
</html>`

// ServeSwaggerUI serves the interactive Swagger UI HTML page.
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}

// ServeOpenAPISpec serves the raw OpenAPI YAML spec embedded at compile time.
func ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "max-age=300")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiSpec)
}
