
rate-limiting.wasm: main.go go.mod 
	tinygo build -o rate-limiting.wasm -scheduler=none -target=wasi ./main.go

clean:
	rm rate-limiting.wasm
