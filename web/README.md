`frontend/` is the upstream cloudflare_temp_email frontend (v1.12.0), unmodified.
`dist/` is its build output and is embedded into the Go binary (`embed.go`).
Rebuild: `cd frontend && pnpm install && npx vite build -m prod --outDir ../dist --emptyOutDir`.
