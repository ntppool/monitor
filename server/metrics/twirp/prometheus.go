// Copyright 2018 Joonas Kuorilehto. All Rights Reserved.
// Copyright 2018 Twitch Interactive, Inc.  All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may not
// use this file except in compliance with the License. A copy of the License is
// located at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// or in the "license" file accompanying this file. This file is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package prometheus is Twirp server hook that collects Prometheus metrics.
package prometheus

import (
	"context"
	"strconv"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	sctx "go.ntppool.org/monitor/server/context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twitchtv/twirp"
)

var (
	requestsReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_requests_total",
			Help: "Number of RPC requests received.",
		},
		[]string{"method", "client", "account", "account_id"},
	)

	responsesSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_responses_total",
			Help: "Number of RPC responses sent.",
		},
		[]string{"method", "status", "client", "account", "account_id"},
	)

	rpcDurations = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "rpc_durations_seconds",
			Help:       "RPC latency distributions.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"method", "status", "client", "account", "account_id"},
	)
)

// MustRegister registers the metrics with the registerer.
// The default registry is used if registerer is nil.
func MustRegister(registerer prometheus.Registerer) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	registerer.MustRegister(requestsReceived)
	registerer.MustRegister(responsesSent)
	registerer.MustRegister(rpcDurations)
}

// NewServerHooks initializes twirp server hooks that record prometheus metrics
// of twirp operations.
//
// The default registry is used if registerer is nil.
func NewServerHooks(registerer prometheus.Registerer) *twirp.ServerHooks {
	MustRegister(registerer)

	hooks := &twirp.ServerHooks{}

	hooks.RequestReceived = func(ctx context.Context) (context.Context, error) {
		ctx = markReqStart(ctx)
		return ctx, nil
	}

	hooks.RequestRouted = func(ctx context.Context) (context.Context, error) {
		method, ok := twirp.MethodName(ctx)
		if !ok {
			return ctx, nil
		}
		client, _ := getReqClient(ctx)
		accountIDToken, accountID := getReqAccount(ctx)
		requestsReceived.WithLabelValues(method, client, accountIDToken, accountID).Inc()
		return ctx, nil
	}

	hooks.ResponseSent = func(ctx context.Context) {
		method, _ := twirp.MethodName(ctx)
		status, _ := twirp.StatusCode(ctx)
		client, _ := getReqClient(ctx)
		accountIDToken, accountID := getReqAccount(ctx)

		responsesSent.WithLabelValues(method, status, client, accountIDToken, accountID).Inc()

		if start, ok := getReqStart(ctx); ok {
			dur := time.Since(start).Seconds()
			rpcDurations.WithLabelValues(method, status, client, accountIDToken, accountID).Observe(dur)
		}
	}
	return hooks
}

var reqStartTimestampKey = new(int)

func markReqStart(ctx context.Context) context.Context {
	return context.WithValue(ctx, reqStartTimestampKey, time.Now())
}

func getReqStart(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(reqStartTimestampKey).(time.Time)
	return t, ok
}

func getReqClient(ctx context.Context) (string, bool) {
	c, ok := ctx.Value(sctx.CertificateKey).(string)
	return c, ok
}

func getReqAccount(ctx context.Context) (string, string) {
	if acc, ok := ctx.Value(sctx.AccountKey).(*ntpdb.Account); ok && acc != nil {
		return acc.IDToken.String, strconv.Itoa(int(acc.ID))
	}
	return "", "0"
}
