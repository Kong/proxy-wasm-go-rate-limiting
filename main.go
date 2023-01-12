package main

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/kong/proxy-wasm-rate-limiting/config"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

// -----------------------------------------------------------------------------
// Utils
// -----------------------------------------------------------------------------

func max(a uint64, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// -----------------------------------------------------------------------------
// Timestamps
// -----------------------------------------------------------------------------

type Timestamps map[string]int64

func getTimestamps(t time.Time) *Timestamps {
	ts := Timestamps{}

	ye, mo, da := t.Year(), t.Month(), t.Day()
	ho, mi, se, lo := t.Hour(), t.Minute(), t.Second(), t.Location()

	ts["now"] = t.UnixMilli()
	ts["second"] = time.Date(ye, mo, da, ho, mi, se, 0, lo).UnixMilli()
	ts["minute"] = time.Date(ye, mo, da, ho, mi, 0, 0, lo).UnixMilli()
	ts["hour"] = time.Date(ye, mo, da, ho, 0, 0, 0, lo).UnixMilli()
	ts["day"] = time.Date(ye, mo, da, 0, 0, 0, 0, lo).UnixMilli()
	ts["month"] = time.Date(ye, mo, 1, 0, 0, 0, 0, lo).UnixMilli()
	ts["year"] = time.Date(ye, 1, 1, 0, 0, 0, 0, lo).UnixMilli()

	return &ts
}

// -----------------------------------------------------------------------------
// VM Context
// -----------------------------------------------------------------------------

type VMContext struct {
	types.DefaultVMContext
}

var expiration map[string]uint64
var xRateLimitLimit map[string]string
var xRateLimitRemaining map[string]string

func (*VMContext) NewPluginContext(vmID uint32) types.PluginContext {
	expiration = map[string]uint64{
		"second": 1,
		"minute": 60,
		"hour":   3600,
		"day":    86400,
		"month":  2592000,
		"year":   31536000,
	}
	
	time.LoadLocation("")

	xRateLimitLimit = make(map[string]string)
	xRateLimitRemaining = make(map[string]string)

	for k, _ := range expiration {
		t := strings.Title(k)

		xRateLimitLimit[k] = "X-RateLimit-Limit-" + t
		xRateLimitRemaining[k] = "X-RateLimit-Remaining-" + t
	}

	return &PluginContext{}
}

// -----------------------------------------------------------------------------
// Plugin Context
// -----------------------------------------------------------------------------

type PluginContext struct {
	types.DefaultPluginContext
	conf config.Config
	limits map[string]int64
}

func (ctx *PluginContext) OnPluginStart(confSize int) types.OnPluginStartStatus {
	data, err := proxywasm.GetPluginConfiguration()
	if err != nil && err != types.ErrorStatusNotFound {
		proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	err = config.Load(data, &ctx.conf)
	if err != nil {
		proxywasm.LogCriticalf("error parsing plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	ctx.limits = map[string]int64{
		"second": ctx.conf.Second,
		"minute": ctx.conf.Minute,
		"hour":   ctx.conf.Hour,
		"day":    ctx.conf.Day,
		"month":  ctx.conf.Month,
		"year":   ctx.conf.Year,
	}

	return types.OnPluginStartStatusOK
}

func (ctx *PluginContext) NewHttpContext(pluginID uint32) types.HttpContext {
	return &RateLimitingContext{
		conf: &ctx.conf,
		limits: &ctx.limits,
	}
}

// -----------------------------------------------------------------------------
// Rate Limiting Context
// -----------------------------------------------------------------------------

type RateLimitingContext struct {
	types.DefaultHttpContext
	conf *config.Config
	limits *map[string]int64
	routeId string
	serviceId string
	headers map[string]string
}

func getForwardedIp() string {
	data, err := proxywasm.GetProperty([]string{"ngx", "remote_addr"})
	if err != nil {
		return string(data)
	}
	return ""
}

func getLocalKey(ctx *RateLimitingContext, id Identifier, period string, date int64) string {
	return fmt.Sprintf("kong_wasm_rate_limiting_counters/ratelimit:%v:%v:%v:%v:%v",
		ctx.routeId, ctx.serviceId, id, date, period)
}

type Identifier string

func getIdentifier(conf *config.Config) Identifier {
	id := ""
	if conf.LimitBy == "header" {
		header, err := proxywasm.GetHttpRequestHeader(conf.HeaderName)
		if err != nil {
			id = header
		}
	} else if conf.LimitBy == "path" {
		reqPath, err := proxywasm.GetHttpRequestHeader(":path")
		if err == nil && reqPath == conf.Path {
			id = reqPath
		}
	}

	if id != "" {
		return Identifier(id)
	}

	// conf.LimitBy == "ip":

	return Identifier(getForwardedIp())
}

type Usage struct {
	limit     int64
	remaining uint64
	usage     uint64
	cas       uint32
}

func localPolicyUsage(ctx *RateLimitingContext, id Identifier, period string, ts *Timestamps) (uint64, uint32, error) {
	cacheKey := getLocalKey(ctx, id, period, (*ts)[period])

	value, cas, err := proxywasm.GetSharedData(cacheKey)
	if err != nil {
		if err == types.ErrorStatusNotFound {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	ret := binary.LittleEndian.Uint64(value)
	return ret, cas, nil
}

func localPolicyIncrement(ctx *RateLimitingContext, id Identifier, counters map[string]Usage, ts *Timestamps) {
	for period, usage := range counters {
		cacheKey := getLocalKey(ctx, id, period, (*ts)[period])
		buf := make([]byte, 8)
		value := usage.usage
		cas := usage.cas

		saved := false
		var err error
		for i := 0; i < 10; i++ {
			binary.LittleEndian.PutUint64(buf, value+1)
			err = proxywasm.SetSharedData(cacheKey, buf, cas)
			if err == nil {
				saved = true
				break
			} else if err == types.ErrorStatusCasMismatch {
				// Get updated value, updated cas and retry
				buf, cas, err = proxywasm.GetSharedData(cacheKey)
				value = binary.LittleEndian.Uint64(buf)
			} else {
				break
			}
		}
		if !saved {
			proxywasm.LogErrorf("could not increment counter for period '%v': %v", period, err)
		}
	}
	// FIXME
}

func getUsage(ctx *RateLimitingContext, id Identifier, ts *Timestamps) (map[string]Usage, string, error) {
	counters := make(map[string]Usage)
	stop := ""

	for period, limit := range *ctx.limits {
		if limit == -1 {
			continue
		}

		curUsage, cas, err := localPolicyUsage(ctx, id, period, ts)
		if err != nil {
			return counters, period, err
		}

		// What is the current usage for the configured limit name?
		remaining := uint64(limit) - curUsage

		// Recording usage
		counters[period] = Usage{
			limit:     limit,
			remaining: remaining,
			usage:     curUsage,
			cas:       cas,
		}

		if remaining <= 0 {
			stop = period
		}
	}

	return counters, stop, nil
}

func processUsage(ctx *RateLimitingContext, counters map[string]Usage, stop string, ts *Timestamps) types.Action {
	conf := ctx.conf
	var headers map[string]string
	reset := uint64(0)

	now := (*ts)["now"]
	if !conf.HideClientHeaders {
		headers = make(map[string]string)
		limit := int64(0)
		window := uint64(0)
		remaining := uint64(0)

		for k, v := range counters {
			curLimit := v.limit
			curWindow := expiration[k]
			curRemaining := v.remaining

			if stop == "" || stop == k {
				curRemaining--
			}
			curRemaining = max(0, curRemaining)

			if (limit == 0) ||
				(curRemaining < remaining) ||
				(curRemaining == remaining && curWindow > window) {

				limit = curLimit
				window = curWindow
				remaining = curRemaining

				reset = max(1, window-uint64((now-((*ts)[k]))/1000))
			}

			headers[xRateLimitLimit[k]] = fmt.Sprintf("%d", limit)
			headers[xRateLimitRemaining[k]] = fmt.Sprintf("%d", remaining)
		}

		headers["RateLimit-Limit"] = fmt.Sprintf("%d", limit)
		headers["RateLimit-Remaining"] = fmt.Sprintf("%d", remaining)
		headers["RateLimit-Reset"] = fmt.Sprintf("%d", reset)
	}

	if stop != "" {
		var pairs [][2]string = nil

		if !conf.HideClientHeaders {
			pairs := [][2]string{}
			if headers != nil {
				for k, v := range headers {
					pairs = append(pairs, [2]string{k, v})
				}
			}
			pairs = append(pairs, [2]string{"Retry-After", fmt.Sprintf("%d", reset)})
		}

		if err := proxywasm.SendHttpResponse(429, pairs, []byte("API rate limit exceeded!"), -1); err != nil {
			panic(err)
		}
		return types.ActionPause
	}
	
	if headers != nil {
		ctx.headers = headers
	}

	return types.ActionContinue
}

func (ctx *RateLimitingContext) OnHttpResponseHeaders(numHeaders int, eof bool) types.Action {
	if !eof {
		return types.ActionContinue
	}
	if ctx.headers != nil {
		pairs, err := proxywasm.GetHttpResponseHeaders()
		if err != nil {
			panic(err)
		}
		for k, v := range ctx.headers {
			pairs = append(pairs, [2]string{k, v})
		}
		proxywasm.ReplaceHttpResponseHeaders(pairs)
	}

	return types.ActionContinue
}

func (ctx *RateLimitingContext) OnHttpRequestHeaders(numHeaders int, eof bool) types.Action {
	ts := getTimestamps(time.Now())

	// Consumer is identified by IP address
	// TODO Add authenticated credential id support
	id := getIdentifier(ctx.conf)

	counters, stop, err := getUsage(ctx, id, ts)
	if err != nil {
		if !ctx.conf.FaultTolerant {
			panic(err)
		}

		proxywasm.LogErrorf("failed to get usage: %v", err)
	}

	if counters != nil {
		action := processUsage(ctx, counters, stop, ts)
		if action != types.ActionContinue {
			return action
		}

		localPolicyIncrement(ctx, id, counters, ts)
	}

	return types.ActionContinue
}

func main() {
	proxywasm.SetVMContext(&VMContext{})
}
