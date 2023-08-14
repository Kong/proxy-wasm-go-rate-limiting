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

## Building and running

Once the environment is set up with `tinygo` and `ffjson` in your PATH,
build the filter running `make`.

Once you have a Wasm-enabled Kong container with a recent ngx_wasm_module
integrated (the container from the Summit 2022 Tech Preview is too old),
you can run the script in `test/demo.sh` to give the filter a spin.

You also need the `kong_wasm_rate_limiting_counters` shared memory
key-value store enabled in your Kong configuration. One way to
achieve this is via the following environment variable:

```sh
export KONG_NGINX_WASM_SHM_KONG_WASM_RATE_LIMITING_COUNTERS=12m
```
