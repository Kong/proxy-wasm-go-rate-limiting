FFJSON=ffjson
TINYGO=tinygo
GOFMT=gofmt

rate-limiting.wasm: main.go config/config.go config/config_ffjson.go go.mod 
	$(TINYGO) build -o rate-limiting.wasm -scheduler=none -target=wasi -tags timetzdata

config/config_ffjson.go: config/config.go
	$(FFJSON) -noencoder config/config.go

fmt:
	$(GOFMT) -w .

clean:
	rm rate-limiting.wasm
