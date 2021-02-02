// Copyright The Jaeger Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package strategy

import (
	"context"

	"github.com/jaegertracing/jaeger-operator/internal/config"

	"go.opentelemetry.io/otel"

	v2 "github.com/jaegertracing/jaeger-operator/apis/jaegertracing/v2"
	"github.com/jaegertracing/jaeger-operator/internal/instrument"
)

// For returns the appropriate Strategy for the given Jaeger instance.
func For(ctx context.Context, cfg config.Config, jaeger v2.Jaeger) Strategy {
	tracer := otel.GetTracerProvider().Tracer(instrument.ReconciliationTracer)
	_, span := tracer.Start(ctx, "strategy.For")
	defer span.End()

	if jaeger.Spec.Strategy == v2.DeploymentStrategyAllInOne {
		return newAllInOneStrategy(ctx, cfg, jaeger)
	}

	return newProductionStrategy(ctx, cfg, jaeger)
}