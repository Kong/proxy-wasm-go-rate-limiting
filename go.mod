module github.com/kong/proxy-wasm-gzip-decompress

go 1.18

require github.com/tetratelabs/proxy-wasm-go-sdk v0.19.0

require (
	github.com/iancoleman/orderedmap v0.0.0-20190318233801-ac98e3ecb4b0 // indirect
	github.com/invopop/jsonschema v0.6.0 // indirect
	github.com/pquerna/ffjson v0.0.0-20190930134022-aa0246cd15f7 // indirect
	github.com/santhosh-tekuri/jsonschema v1.2.4 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.0.2 // indirect
	github.com/tidwall/gjson v1.14.3 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
)

replace github.com/kong/proxy-wasm-rate-limiting/schema => ../schema
