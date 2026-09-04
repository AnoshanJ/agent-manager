// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// failureSummarySvc builds the service under test over repo. gatewayApplier is
// nil, which is the open-source shape and irrelevant to a read.
func failureSummarySvc(repo repositories.GatewayRepository) *PlatformGatewayService {
	return NewPlatformGatewayService(repo, nil)
}

// summaryQuery is a valid query with both thresholds set, for tests that are
// not about the thresholds themselves.
func summaryQuery(includeDetails bool) GatewayFailureSummaryQuery {
	return GatewayFailureSummaryQuery{
		StalenessThreshold:         5 * time.Minute,
		FailurePercentageThreshold: 10,
		IncludeDetails:             includeDetails,
	}
}

// TestGatewayFailureSummary_CountsOnly is the default response: the repository's
// counts pass through untouched, and the threshold used is reported alongside
// them so the number is interpretable.
//
// ListFailedGatewaysAllOrgsFunc is left nil on purpose — an unconfigured moq
// method panics, so this also asserts the detail query is not run when it was
// not asked for.
func TestGatewayFailureSummary_CountsOnly(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, _ time.Time,
		) (repositories.GatewayFailureCounts, error) {
			return repositories.GatewayFailureCounts{Total: 412, Failed: 7}, nil
		},
	}

	resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
		context.Background(), summaryQuery(false))

	require.NoError(t, err)
	assert.Equal(t, int64(412), resp.Total)
	assert.Equal(t, int64(7), resp.Failed)
	assert.Equal(t, 300, resp.StalenessThresholdSeconds)
	assert.Equal(t, float64(10), resp.FailurePercentageThreshold)
	assert.False(t, resp.Truncated)
	assert.Nil(t, resp.FailedGateways, "details must be omitted when not requested")
	assert.False(t, resp.EvaluatedAt.IsZero(), "evaluatedAt must name the instant the counts describe")
}

// TestGatewayFailureSummary_StaleBeforeIsNowMinusThreshold pins the arithmetic
// the whole endpoint rests on. Passing now+threshold, or the threshold itself,
// would return a plausible-looking number that answers a different question.
func TestGatewayFailureSummary_StaleBeforeIsNowMinusThreshold(t *testing.T) {
	const threshold = 5 * time.Minute
	var got time.Time

	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, staleBefore time.Time,
		) (repositories.GatewayFailureCounts, error) {
			got = staleBefore
			return repositories.GatewayFailureCounts{Total: 1, Failed: 0}, nil
		},
	}

	before := time.Now()
	resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
		context.Background(), GatewayFailureSummaryQuery{
			StalenessThreshold:         threshold,
			FailurePercentageThreshold: 10,
			IncludeDetails:             false,
		})
	after := time.Now()

	require.NoError(t, err)
	assert.False(t, got.Before(before.Add(-threshold)),
		"staleBefore is older than the call's own start minus the threshold")
	assert.False(t, got.After(after.Add(-threshold)),
		"staleBefore is newer than the call's own return minus the threshold")
	// The cutoff and the reported instant must be exactly one threshold apart,
	// or the response describes a window it did not query.
	assert.WithinDuration(t, resp.EvaluatedAt.Add(-threshold), got, time.Millisecond)
}

// TestGatewayFailureSummary_RejectsInvalidThresholds refuses thresholds that
// cannot produce a meaningful verdict, and does so before touching the database
// (both mock funcs are left nil, so any query panics).
//
// The percentage bounds are the load-bearing half: at or below 0 every fleet is
// unhealthy including an empty one, and above 100 none ever is. Either turns the
// verdict into a constant, which is worse than an error because it still looks
// like an answer.
func TestGatewayFailureSummary_RejectsInvalidThresholds(t *testing.T) {
	svc := failureSummarySvc(&repomocks.GatewayRepositoryMock{})

	for _, tc := range []struct {
		name  string
		query GatewayFailureSummaryQuery
	}{
		{"zero staleness", GatewayFailureSummaryQuery{
			StalenessThreshold: 0, FailurePercentageThreshold: 10, IncludeDetails: false,
		}},
		{"negative staleness", GatewayFailureSummaryQuery{
			StalenessThreshold: -time.Minute, FailurePercentageThreshold: 10, IncludeDetails: false,
		}},
		{"zero percentage", GatewayFailureSummaryQuery{
			StalenessThreshold: time.Minute, FailurePercentageThreshold: 0, IncludeDetails: false,
		}},
		{"negative percentage", GatewayFailureSummaryQuery{
			StalenessThreshold: time.Minute, FailurePercentageThreshold: -1, IncludeDetails: false,
		}},
		{"percentage over 100", GatewayFailureSummaryQuery{
			StalenessThreshold: time.Minute, FailurePercentageThreshold: 100.01, IncludeDetails: false,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.GetCrossOrgGatewayFailureSummary(context.Background(), tc.query)
			assert.ErrorIs(t, err, utils.ErrBadRequest)
		})
	}
}

// TestGatewayFailureSummary_PropagatesCountError is the error-masking guard. A
// failed query must surface as an error, never as a zero-count response — an
// alert reading total=0 failed=0 would conclude the fleet is fine.
func TestGatewayFailureSummary_PropagatesCountError(t *testing.T) {
	sentinel := errors.New("connection refused")
	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, _ time.Time,
		) (repositories.GatewayFailureCounts, error) {
			return repositories.GatewayFailureCounts{}, sentinel
		},
	}

	resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
		context.Background(), summaryQuery(false))

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "the underlying error must stay unwrappable")
	assert.NotErrorIs(t, err, utils.ErrGatewayNotFound, "a real error must not be flattened into not-found")
	assert.Nil(t, resp)
}

// TestGatewayFailureSummary_PropagatesDetailError covers the second query
// failing after the first succeeded. Returning the counts with the list silently
// missing would read as "nothing to name", which is the opposite of what
// happened.
func TestGatewayFailureSummary_PropagatesDetailError(t *testing.T) {
	sentinel := errors.New("statement timeout")
	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, _ time.Time,
		) (repositories.GatewayFailureCounts, error) {
			return repositories.GatewayFailureCounts{Total: 10, Failed: 3}, nil
		},
		ListFailedGatewaysAllOrgsFunc: func(
			_ context.Context, _ time.Time, _ int,
		) ([]*models.Gateway, error) {
			return nil, sentinel
		},
	}

	resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
		context.Background(), summaryQuery(true))

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Nil(t, resp)
}

// TestGatewayFailureSummary_IncludeDetails checks the detail rows carry the
// organization. The caller is cross-org, so a name without an owner does not
// identify a gateway.
func TestGatewayFailureSummary_IncludeDetails(t *testing.T) {
	id := uuid.New()
	lastUpdated := time.Now().Add(-42 * time.Minute)

	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, _ time.Time,
		) (repositories.GatewayFailureCounts, error) {
			return repositories.GatewayFailureCounts{Total: 10, Failed: 1}, nil
		},
		ListFailedGatewaysAllOrgsFunc: func(
			_ context.Context, _ time.Time, limit int,
		) ([]*models.Gateway, error) {
			// One over the cap, so a full page is distinguishable from a page
			// that ends exactly at it.
			assert.Equal(t, maxFailedGatewayDetailRows+1, limit)
			return []*models.Gateway{
				{UUID: id, Name: "prod-ingress", OUID: "ou-abc", UpdatedAt: lastUpdated},
			}, nil
		},
	}

	resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
		context.Background(), summaryQuery(true))

	require.NoError(t, err)
	assert.False(t, resp.Truncated)
	require.Len(t, resp.FailedGateways, 1)
	assert.Equal(t, id.String(), resp.FailedGateways[0].ID)
	assert.Equal(t, "prod-ingress", resp.FailedGateways[0].Name)
	assert.Equal(t, "ou-abc", resp.FailedGateways[0].OrganizationID)
	assert.Equal(t, lastUpdated, resp.FailedGateways[0].LastUpdatedAt)
}

// TestGatewayFailureSummary_TruncatesDetails is why the cap is fetched as
// limit+1. A capped list must say so: without Truncated the response reads as
// the complete set of failures, and Failed would silently disagree with it.
func TestGatewayFailureSummary_TruncatesDetails(t *testing.T) {
	overflowing := make([]*models.Gateway, 0, maxFailedGatewayDetailRows+1)
	for i := 0; i <= maxFailedGatewayDetailRows; i++ {
		overflowing = append(overflowing, &models.Gateway{
			UUID: uuid.New(), Name: fmt.Sprintf("gw-%d", i), OUID: "ou-abc", UpdatedAt: time.Now(),
		})
	}

	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, _ time.Time,
		) (repositories.GatewayFailureCounts, error) {
			return repositories.GatewayFailureCounts{
				Total: 9000, Failed: int64(len(overflowing)),
			}, nil
		},
		ListFailedGatewaysAllOrgsFunc: func(
			_ context.Context, _ time.Time, _ int,
		) ([]*models.Gateway, error) {
			return overflowing, nil
		},
	}

	resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
		context.Background(), summaryQuery(true))

	require.NoError(t, err)
	assert.True(t, resp.Truncated, "a capped list must be reported as capped")
	assert.Len(t, resp.FailedGateways, maxFailedGatewayDetailRows)
	assert.Equal(t, int64(maxFailedGatewayDetailRows+1), resp.Failed,
		"Failed stays the authoritative count even when the list is capped")
}

// TestGatewayFailureSummary_PercentageAndVerdict is the arithmetic and the
// verdict together, since the response has to be self-consistent: the
// percentage it reports must be the one the verdict was taken on.
//
// The boundary rows are the point. The threshold is inclusive — at exactly the
// threshold the fleet is unhealthy — and the value compared is the rounded one,
// so a fleet that rounds up to the threshold is reported unhealthy rather than
// answering "10% failing, healthy: true".
func TestGatewayFailureSummary_PercentageAndVerdict(t *testing.T) {
	for _, tc := range []struct {
		name           string
		total, failed  int64
		threshold      float64
		wantPercentage float64
		wantHealthy    bool
	}{
		{"well under the threshold", 412, 7, 10, 1.7, true},
		{"nothing failing", 100, 0, 10, 0, true},
		{"just under the threshold", 1000, 99, 10, 9.9, true},
		{"exactly at the threshold is unhealthy", 100, 10, 10, 10, false},
		{"over the threshold", 100, 40, 10, 40, false},
		{"whole fleet down", 8, 8, 10, 100, false},
		{"sub-percent threshold catches a large fleet", 10000, 60, 0.5, 0.6, false},
		{"rounded to two places", 3, 1, 50, 33.33, true},
		// 9999/100000 is 9.999% raw, which rounds to exactly 10.00. The
		// reported number and the verdict must agree, which is why the
		// comparison uses the rounded value: against the raw quotient
		// 9.999 < 10 would report this fleet healthy while printing "10".
		{"rounds up onto the threshold", 100000, 9999, 10, 10, false},
		// An empty fleet: nothing is failing, and the division must not happen.
		{"empty fleet is healthy", 0, 0, 10, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &repomocks.GatewayRepositoryMock{
				CountFailureSummaryAllOrgsFunc: func(
					_ context.Context, _ time.Time,
				) (repositories.GatewayFailureCounts, error) {
					return repositories.GatewayFailureCounts{Total: tc.total, Failed: tc.failed}, nil
				},
			}

			resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
				context.Background(), GatewayFailureSummaryQuery{
					StalenessThreshold:         5 * time.Minute,
					FailurePercentageThreshold: tc.threshold,
					IncludeDetails:             false,
				})

			require.NoError(t, err)
			assert.Equal(t, tc.wantPercentage, resp.FailurePercentage)
			assert.Equal(t, tc.wantHealthy, resp.Healthy)
			// The verdict must be exactly what the reported numbers imply, so a
			// reader of the payload can never disagree with it.
			assert.Equal(t, resp.FailurePercentage < resp.FailurePercentageThreshold, resp.Healthy)
		})
	}
}

// TestGatewayFailureSummary_VerdictIsPresentWithDetails pins that asking for
// details does not cost you the verdict.
//
// The controller answers 200 to a detailed request whatever the fleet looks
// like, so "healthy" in the body is the only channel left. A detailed response
// that omitted or defaulted it would silently report every degraded fleet as
// fine to exactly the caller looking closest.
func TestGatewayFailureSummary_VerdictIsPresentWithDetails(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, _ time.Time,
		) (repositories.GatewayFailureCounts, error) {
			return repositories.GatewayFailureCounts{Total: 10, Failed: 9}, nil
		},
		ListFailedGatewaysAllOrgsFunc: func(
			_ context.Context, _ time.Time, _ int,
		) ([]*models.Gateway, error) {
			return []*models.Gateway{
				{UUID: uuid.New(), Name: "gw-down", OUID: "ou-abc", UpdatedAt: time.Now().Add(-time.Hour)},
			}, nil
		},
	}

	resp, err := failureSummarySvc(repo).GetCrossOrgGatewayFailureSummary(
		context.Background(), summaryQuery(true))

	require.NoError(t, err)
	assert.False(t, resp.Healthy, "a 90%-failed fleet must report unhealthy even in the detailed form")
	assert.Equal(t, float64(90), resp.FailurePercentage)
	assert.Len(t, resp.FailedGateways, 1)
}
