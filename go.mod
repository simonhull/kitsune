module github.com/simonhull/kitsune

go 1.27.0

// npm packages can ship .go files (flatted does); ./... must not find them.
ignore ./web/node_modules

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/ncruces/go-sqlite3 v0.36.0
	github.com/pb33f/libopenapi v0.41.8
	github.com/pb33f/libopenapi-validator v0.16.2
	github.com/simonhull/audiometa v0.12.0
	golang.org/x/crypto v0.58.0
	golang.org/x/image v0.47.0
	golang.org/x/term v0.47.0
	golang.org/x/text v0.43.0
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/basgys/goxml2json v1.1.1-0.20231018121955-e66ee54ceaad // indirect
	github.com/buger/jsonparser v1.6.1 // indirect
	github.com/go-openapi/jsonpointer v1.0.2 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.4.35401 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/pb33f/go-yaml v0.1.1 // indirect
	github.com/pb33f/jsonpath v0.8.5 // indirect
	github.com/pb33f/jsonschema/v6 v6.0.3 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.2 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)
