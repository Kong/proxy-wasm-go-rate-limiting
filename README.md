# proxy-wasm-go-rate-limiting

A prototype implementation of a rate-limiting filter written in Go,
using the proxy-wasm API for running on WebAssembly-enabled gateways.

## What's implemented

* "local" policy only, using the SHM-based key-value store

## What's missing

* Getting proper route and service ids for producing identifiers.
* Other policies, which would require additional features from the
  underlying system, such as calling out to a Redis instance.

## Build requirements

* [tinygo](https://tinygo.org)
* [proxy-wasm-go-sdk](github.com/tetratelabs/proxy-wasm-go-sdk)
* [ffjson](https://github.com/pquerna/ffjson)

## Building and running

Once the environment is set up with `tinygo` and `ffjson` in your PATH,
build the filter running `make`.

Once you have a Wasm-enabled Kong container with a recent ngx_wasm_module
integrated (the container from the Summit 2022 Tech Preview is too old),
you can run the script in `test/demo.sh` to give the filter a spin.
