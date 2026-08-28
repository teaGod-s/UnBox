# M4 Web Playback Verification

Branch: `feat/m4-web-playback`

Verified on Linux:

- `env GOCACHE=/tmp/unbox-m4-go-cache go test ./... -count=1`
- `env GOCACHE=/tmp/unbox-m4-vet-cache go vet ./...`
- `test -z "$(gofmt -l .)"`
- `CGO_ENABLED=1 GOCACHE=/tmp/unbox-m4-cgo-cache go build ./...`
- `cd frontend && npm test`
- `cd frontend && npm run build`
- `git diff --check`
- Go LSP diagnostics: no errors or warnings on changed Go files.

The frontend LSP bridge could not initialize because it did not locate the worktree TypeScript installation; `vue-tsc` passed as part of the production build.

Native Windows and macOS playback, installer execution, and real WebView media rendering still require their respective host machines. Windows installer metadata is pinned to mpv-distributions `0.41.0` x86_64 with SHA-256 `1b32d5eb7e713ecc5853c18107daffac652e29474dfd517a4ddb792dc45e40fc`.
