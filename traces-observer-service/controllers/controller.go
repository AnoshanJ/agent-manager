// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package controllers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

// ErrTraceNotFound is returned when a trace is not found
var ErrTraceNotFound = errors.New("trace not found")

const (
	// MaxSpansPerRequest is the maximum number of spans that can be fetched in a single query
	MaxSpansPerRequest = 10000
	// MaxTracesPerRequest is the maximum number of traces that can be requested at once
	MaxTracesPerRequest = 1000
	// DefaultTracesLimit is the default number of traces to return when no limit is specified
	DefaultTracesLimit = 10
)

// TracingController provides tracing functionality
type TracingController struct {
	osClient *opensearch.Client
}

// NewTracingController creates a new tracing service
func NewTracingController(osClient *opensearch.Client) *TracingController {
	return &TracingController{
		osClient: osClient,
	}
}

// GetTraceById retrieves spans for a specific trace.
// When params.ParentSpan is true, only the root span (parentSpanId == "") is returned.
func (s *TracingController) GetTraceById(ctx context.Context, params opensearch.TraceByIdParams) (*opensearch.TraceResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace by ID",
		"traceIds", params.TraceIDs,
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"parentSpan", params.ParentSpan)

	// Build query
	query := opensearch.BuildTraceByIdsQuery(params)

	// Resolve indices from time range, or search all if no time range provided
	var indices []string
	var err error
	if params.StartTime != "" && params.EndTime != "" {
		indices, err = opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
		if err != nil {
			return nil, fmt.Errorf("failed to generate indices: %w", err)
		}
	} else {
		indices = opensearch.GetAllTraceIndices()
	}

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search traces: %w", err)
	}

	// Parse spans
	spans := opensearch.ParseSpans(response)

	if len(spans) == 0 {
		log.Warn("No spans found for trace",
			"traceIds", params.TraceIDs,
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid)
		return nil, ErrTraceNotFound
	}

	// Extract token usage and status from returned spans
	tokenUsage := opensearch.ExtractTokenUsage(spans)
	traceStatus := opensearch.ExtractTraceStatus(spans)

	log.Info("Retrieved trace spans",
		"span_count", len(spans),
		"traceIds", params.TraceIDs,
		"parentSpan", params.ParentSpan)

	return &opensearch.TraceResponse{
		Spans:      spans,
		TotalCount: len(spans),
		TokenUsage: tokenUsage,
		Status:     traceStatus,
	}, nil
}

// GetTraceOverviews retrieves trace overviews using search_after on root spans
// for deterministic cursor-based pagination, then enriches with span count data.
func (s *TracingController) GetTraceOverviews(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace overviews",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"limit", params.Limit,
		"hasCursor", params.AfterCursor != nil)

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = DefaultTracesLimit
	}
	if params.Limit > MaxTracesPerRequest {
		params.Limit = MaxTracesPerRequest
	}

	// Phase 1: Search root spans with search_after for deterministic pagination
	query := opensearch.BuildRootSpanSearchQuery(params)

	var indices []string
	var err error
	if params.StartTime != "" && params.EndTime != "" {
		indices, err = opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
		if err != nil {
			return nil, fmt.Errorf("failed to generate indices: %w", err)
		}
	} else {
		indices = opensearch.GetAllTraceIndices()
	}

	response, err := s.osClient.SearchRootSpans(ctx, indices, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search root spans: %w", err)
	}

	totalCount := response.Aggregations.TotalTraces.Value
	rootSpans := opensearch.ParseRootSpanHits(response)

	if len(rootSpans) == 0 {
		return &opensearch.TraceOverviewResponse{
			Traces:     []opensearch.TraceOverview{},
			TotalCount: totalCount,
		}, nil
	}

	// Collect trace IDs for span count query
	traceIDs := make([]string, len(rootSpans))
	for i, span := range rootSpans {
		traceIDs[i] = span.TraceID
	}

	// Phase 2: Get span counts for the traces on this page
	spanCountMap := make(map[string]int, len(traceIDs))
	spanCountQuery := opensearch.BuildSpanCountQuery(traceIDs, params)
	spanCountResponse, err := s.osClient.SearchSpanCounts(ctx, indices, spanCountQuery)
	if err != nil {
		log.Warn("Failed to fetch span counts, using 0", "error", err)
	} else {
		for _, bucket := range spanCountResponse.Aggregations.Traces.Buckets {
			spanCountMap[bucket.Key] = bucket.DocCount
		}
	}

	// Build trace overviews in search result order (preserves sort)
	overviews := make([]opensearch.TraceOverview, 0, len(rootSpans))
	for i := range rootSpans {
		rootSpan := &rootSpans[i]

		// Extract input/output from root span
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Extract token usage from root span's traceloop.entity.output,
		// falling back to gen_ai.usage.* attributes
		tokenUsage := opensearch.ExtractTokenUsageFromEntityOutput(rootSpan)
		if tokenUsage == nil {
			tokenUsage = opensearch.ExtractTokenUsage([]opensearch.Span{*rootSpan})
		}
		traceStatus := opensearch.ExtractTraceStatus([]opensearch.Span{*rootSpan})

		overviews = append(overviews, opensearch.TraceOverview{
			TraceID:         rootSpan.TraceID,
			RootSpanID:      rootSpan.SpanID,
			RootSpanName:    rootSpan.Name,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       rootSpan.StartTime.Format(time.RFC3339Nano),
			EndTime:         rootSpan.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: rootSpan.DurationInNanos,
			SpanCount:       spanCountMap[rootSpan.TraceID],
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
		})
	}

	// Compute next cursor from the last hit's sort values
	var nextCursor *opensearch.PaginationCursor
	if len(response.Hits.Hits) == params.Limit {
		lastHit := response.Hits.Hits[len(response.Hits.Hits)-1]
		if len(lastHit.Sort) >= 2 {
			nextCursor = &opensearch.PaginationCursor{
				StartTime: fmt.Sprintf("%v", lastHit.Sort[0]),
				TraceID:   fmt.Sprintf("%v", lastHit.Sort[1]),
			}
		}
	}

	log.Info("Retrieved trace overviews",
		"totalCount", totalCount,
		"returned", len(overviews),
		"limit", params.Limit,
		"hasNextCursor", nextCursor != nil)

	return &opensearch.TraceOverviewResponse{
		Traces:     overviews,
		TotalCount: totalCount,
		NextCursor: nextCursor,
	}, nil
}

// ExportTraces retrieves complete trace objects with all spans for export.
// Uses search_after on root spans for trace discovery, then fetches all spans per trace.
func (s *TracingController) ExportTraces(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceExportResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Starting trace export",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"limit", params.Limit,
		"hasCursor", params.AfterCursor != nil)

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = DefaultTracesLimit
	}
	if params.Limit > MaxTracesPerRequest {
		params.Limit = MaxTracesPerRequest
	}

	// Phase 1: Search root spans with search_after for deterministic pagination
	query := opensearch.BuildRootSpanSearchQuery(params)

	indices, err := opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	response, err := s.osClient.SearchRootSpans(ctx, indices, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search root spans for export: %w", err)
	}

	totalCount := response.Aggregations.TotalTraces.Value
	rootSpans := opensearch.ParseRootSpanHits(response)

	if len(rootSpans) == 0 {
		return &opensearch.TraceExportResponse{
			Traces:     []opensearch.FullTrace{},
			TotalCount: totalCount,
		}, nil
	}

	// Collect trace IDs from root spans
	traceIDs := make([]string, len(rootSpans))
	for i, span := range rootSpans {
		traceIDs[i] = span.TraceID
	}

	// Get span counts for these traces
	spanCountMap := make(map[string]int, len(traceIDs))
	spanCountQuery := opensearch.BuildSpanCountQuery(traceIDs, params)
	spanCountResponse, err := s.osClient.SearchSpanCounts(ctx, indices, spanCountQuery)
	if err != nil {
		log.Warn("Failed to fetch span counts for export, using 0", "error", err)
	} else {
		for _, bucket := range spanCountResponse.Aggregations.Traces.Buckets {
			spanCountMap[bucket.Key] = bucket.DocCount
		}
	}

	// Calculate total span count for fetch limit
	totalSpanCount := 0
	for _, count := range spanCountMap {
		totalSpanCount += count
	}

	// Cap at OpenSearch max_result_window default
	truncated := false
	if totalSpanCount > MaxSpansPerRequest {
		log.Warn("Span count exceeds maximum, export will be truncated",
			"requestedSpans", totalSpanCount,
			"maxSpans", MaxSpansPerRequest)
		totalSpanCount = MaxSpansPerRequest
		truncated = true
	}
	if totalSpanCount == 0 {
		totalSpanCount = len(traceIDs) * 10 // fallback estimate
	}

	// Phase 2: Fetch ALL spans for each trace (no parentSpan filter)
	allSpansParams := opensearch.TraceByIdParams{
		TraceIDs:       traceIDs,
		ComponentUid:   params.ComponentUid,
		EnvironmentUid: params.EnvironmentUid,
		ParentSpan:     false,
		Limit:          totalSpanCount,
	}

	allSpansQuery := opensearch.BuildTraceByIdsQuery(allSpansParams)
	allSpansResponse, err := s.osClient.Search(ctx, indices, allSpansQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spans for export: %w", err)
	}

	allSpans := opensearch.ParseSpans(allSpansResponse)

	// Group spans by traceId
	spansByTrace := make(map[string][]opensearch.Span, len(traceIDs))
	for _, span := range allSpans {
		spansByTrace[span.TraceID] = append(spansByTrace[span.TraceID], span)
	}

	// Build FullTrace objects in root span search order (preserves sort)
	fullTraces := make([]opensearch.FullTrace, 0, len(rootSpans))
	for i := range rootSpans {
		traceID := rootSpans[i].TraceID
		traceSpans, ok := spansByTrace[traceID]
		if !ok || len(traceSpans) == 0 {
			log.Warn("No spans found for trace in export, skipping",
				"traceId", traceID)
			continue
		}

		// Sort spans by start time
		sort.Slice(traceSpans, func(a, b int) bool {
			return traceSpans[a].StartTime.Before(traceSpans[b].StartTime)
		})

		// Find root span from all spans (more reliable than using the search hit)
		var rootSpan *opensearch.Span
		for j := range traceSpans {
			if traceSpans[j].ParentSpanID == "" {
				rootSpan = &traceSpans[j]
				break
			}
		}

		if rootSpan == nil {
			log.Warn("No root span found for trace in export, skipping",
				"traceId", traceID)
			continue
		}

		// Extract input/output from root span
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Token usage and status from all spans
		tokenUsage := opensearch.ExtractTokenUsage(traceSpans)
		traceStatus := opensearch.ExtractTraceStatus(traceSpans)

		// Extract task.id and trial.id from baggage attributes
		var taskId, trialId string
		if rootSpan.Attributes != nil {
			if v, ok := rootSpan.Attributes["task.id"].(string); ok {
				taskId = v
			}
			if v, ok := rootSpan.Attributes["trial.id"].(string); ok {
				trialId = v
			}
		}

		fullTraces = append(fullTraces, opensearch.FullTrace{
			TraceID:         traceID,
			RootSpanID:      rootSpan.SpanID,
			RootSpanName:    rootSpan.Name,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       rootSpan.StartTime.Format(time.RFC3339Nano),
			EndTime:         rootSpan.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: rootSpan.DurationInNanos,
			SpanCount:       spanCountMap[traceID],
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
			TaskId:          taskId,
			TrialId:         trialId,
			Spans:           traceSpans,
		})
	}

	// Compute next cursor from the last hit's sort values
	var nextCursor *opensearch.PaginationCursor
	if len(response.Hits.Hits) == params.Limit {
		lastHit := response.Hits.Hits[len(response.Hits.Hits)-1]
		if len(lastHit.Sort) >= 2 {
			nextCursor = &opensearch.PaginationCursor{
				StartTime: fmt.Sprintf("%v", lastHit.Sort[0]),
				TraceID:   fmt.Sprintf("%v", lastHit.Sort[1]),
			}
		}
	}

	log.Info("Successfully completed trace export",
		"exportedTraces", len(fullTraces),
		"totalCount", totalCount,
		"limit", params.Limit,
		"hasNextCursor", nextCursor != nil)

	return &opensearch.TraceExportResponse{
		Traces:     fullTraces,
		TotalCount: totalCount,
		Truncated:  truncated,
		NextCursor: nextCursor,
	}, nil
}

// HealthCheck checks if the service is healthy
func (s *TracingController) HealthCheck(ctx context.Context) error {
	return s.osClient.HealthCheck(ctx)
}
