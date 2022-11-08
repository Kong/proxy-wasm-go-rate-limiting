package main

import (
	"strings"
	"time"

	"github.com/kong/proxy-wasm-rate-limiting/config"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

// -----------------------------------------------------------------------------
// VM Context
// -----------------------------------------------------------------------------

type VMContext struct {
	types.DefaultVMContext
}

var expiration map[string]int64
var xRateLimitLimit map[string]string
var xRateLimitRemaining map[string]string

func (*VMContext) NewPluginContext(vmID uint32) types.PluginContext {
	expiration = map[string]int64{
		"second": 1,
		"minute": 60,
		"hour":   3600,
		"day":    86400,
		"month":  2592000,
		"year":   31536000,
	}

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

	return types.OnPluginStartStatusOK
}

func (ctx *PluginContext) NewHttpContext(pluginID uint32) types.HttpContext {
	return &RateLimitingContext{
		conf: &ctx.conf,
	}
}

// -----------------------------------------------------------------------------
// Rate Limiting Context
// -----------------------------------------------------------------------------

type RateLimitingContext struct {
	types.DefaultHttpContext
	conf *config.Config
}

func getForwardedIp() string {
	data, err := proxywasm.GetProperty([]string{"ngx", "remote_addr"})
	if err != nil {
		return string(data)
	}
	return ""
}

func getIdentifier(conf *config.Config) string {
	identifier := ""
	if conf.LimitBy == "header" {
		header, err := proxywasm.GetHttpRequestHeader(conf.HeaderName)
		if err != nil {
			identifier = header
		}
	} else if conf.LimitBy == "path" {
		reqPath, err := proxywasm.GetHttpRequestHeader(":path")
		if err == nil && reqPath == conf.Path {
			identifier = reqPath
		}
	}

	if identifier != "" {
		return identifier
	}

	// conf.LimitBy == "ip":

	return getForwardedIp()
}

type Usage struct {
	limit     int64
	remaining int64
}

func localPolicyUsage(conf *config.Config, identifier string, period string, now time.Time) (int64, error) {
	// FIXME
	return 0, nil
}

func localPolicyIncrement(conf *config.Config, limits map[string]int64, identifier string, now time.Time) {
	// FIXME
}

func getUsage(conf *config.Config, identifier string, now time.Time, limits map[string]int64) (map[string]Usage, string, error) {
	usage := make(map[string]Usage)
	stop := ""

	for period, limit := range limits {
		if limit == -1 {
			continue
		}

		curUsage, err := localPolicyUsage(conf, identifier, period, now)
		if err != nil {
			return usage, period, err
		}

		// What is the current usage for the configured limit name?
		remaining := limit - curUsage

		// Recording usage
		usage[period] = Usage{
			limit:     limit,
			remaining: remaining,
		}

		if remaining <= 0 {
			stop = period
		}
	}

	return usage, stop, nil
}

func max(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type Stamps map[string]int64

func getTimestamps(now int64) *Stamps {
	stamps := Stamps{}

	t := time.UnixMilli(now)

	ye, mo, da, ho, mi, se, lo := t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Location()

	stamps["second"] = time.Date(ye, mo, da, ho, mi, se, 0, lo).UnixMilli()
	stamps["minute"] = time.Date(ye, mo, da, ho, mi, 0, 0, lo).UnixMilli()
	stamps["hour"] = time.Date(ye, mo, da, ho, 0, 0, 0, lo).UnixMilli()
	stamps["day"] = time.Date(ye, mo, da, 0, 0, 0, 0, lo).UnixMilli()
	stamps["month"] = time.Date(ye, mo, 1, 0, 0, 0, 0, lo).UnixMilli()
	stamps["year"] = time.Date(ye, 1, 1, 0, 0, 0, 0, lo).UnixMilli()

	return &stamps
}

func processUsage(conf *config.Config, usage map[string]Usage, stop string, now time.Time) types.Action {
	var headers map[string]string
	reset := int64(0)

	curTimestamp := now.UnixMilli()
	if !conf.HideClientHeaders {
		headers = make(map[string]string)
		limit := int64(0)
		window := int64(0)
		remaining := int64(0)
		var timestamps *Stamps

		for k, v := range usage {
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

				if timestamps == nil {
					timestamps = getTimestamps(curTimestamp)
				}

				reset = max(1, window-((curTimestamp-(*timestamps)[k])/1000))
			}

			headers[xRateLimitLimit[k]] = string(limit)
			headers[xRateLimitRemaining[k]] = string(remaining)
		}

		headers["RateLimit-Limit"] = string(limit)
		headers["RateLimit-Remaining"] = string(remaining)
		headers["RateLimit-Reset"] = string(reset)
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
			pairs = append(pairs, [2]string{"Retry-After", string(reset)})
		}

		if err := proxywasm.SendHttpResponse(429, pairs, []byte("API rate limit exceeded!"), -1); err != nil {
			panic(err)
		}
		return types.ActionPause
	}

	return types.ActionContinue
}

func (ctx *RateLimitingContext) OnHttpRequestHeaders(numHeaders int, eof bool) types.Action {
	now := time.Now()

	// Consumer is identified by IP address
	// TODO Add authenticated credential id support
	identifier := getIdentifier(ctx.conf)

	limits := map[string]int64{
		"second": ctx.conf.Second,
		"minute": ctx.conf.Minute,
		"hour":   ctx.conf.Hour,
		"day":    ctx.conf.Day,
		"month":  ctx.conf.Month,
		"year":   ctx.conf.Year,
	}

	usage, stop, err := getUsage(ctx.conf, identifier, now, limits)
	if err != nil {
		if !ctx.conf.FaultTolerant {
			panic(err)
		}

		proxywasm.LogErrorf("failed to get usage: %v", err)
	}

	if usage != nil {
		action := processUsage(ctx.conf, usage, stop, now)
		if action != types.ActionContinue {
			return action
		}
	}

	localPolicyIncrement(ctx.conf, limits, identifier, now)

	return types.ActionContinue
}

func main() {
	proxywasm.SetVMContext(&VMContext{})
}
