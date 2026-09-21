# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""Monitor eligibility across instrumentation conventions and evaluation levels."""

import pytest

from amp_evaluation import evaluator, EvalResult
from amp_evaluation.runner import Monitor, Experiment
from amp_evaluation.trace import Trace, parse_trace_for_evaluation
from amp_evaluation.trace.models import AgentSpan, AgentTrace, LLMSpan
from amp_evaluation.trace.fetcher import OTELTrace, OTELSpan, OTELTraceStatus, AmpAttributes, AmpSpanStatus


def span(span_id="root", kind="agent", operation=None, name="agent.run", parent=None, **kwargs):
    return OTELSpan(
        traceId="trace",
        spanId=span_id,
        name=name,
        service="test",
        startTime="2026-01-01T00:00:00Z",
        endTime="2026-01-01T00:00:01Z",
        durationInNanos=1000000000,
        status=kwargs.pop("status", "OK"),
        parentSpanId=parent,
        attributes=kwargs.pop("attributes", {"gen_ai.operation.name": operation} if operation else {}),
        ampAttributes=AmpAttributes(kind=kind, status=kwargs.pop("amp_status", AmpSpanStatus())),
    )


def parse(spans, error_count=0):
    return parse_trace_for_evaluation(
        OTELTrace(
            traceId="trace",
            rootSpanId="root",
            rootSpanName=spans[0].name,
            startTime=spans[0].startTime,
            endTime=spans[0].endTime,
            spans=spans,
            status=OTELTraceStatus(errorCount=error_count),
        )
    )


@pytest.mark.parametrize(
    "operation,name,expected",
    [
        ("create_agent", "arbitrary", True),
        (None, "create_agent", True),
        (None, "create_agent WeatherAgent", True),
        ("invoke_agent", "create_agent WeatherAgent", False),
        (None, "weather.agent", False),
        (None, "my_create_agent_workflow", False),
        (None, "create_agent_custom", False),
    ],
)
def test_operation_semantics(operation, name, expected):
    trace = parse([span(operation=operation, name=name)])
    assert trace.initialization_only is expected
    assert (trace.get_agents()[0].operation_name == "create_agent") is expected


def test_initialization_under_infrastructure_and_mixed_execution():
    spans = [span(kind="chain"), span("init", operation="create_agent", parent="root")]
    assert parse(spans).initialization_only
    spans.append(span("call", kind="llm", parent="root"))
    assert not parse(spans).initialization_only


@pytest.mark.parametrize(
    "root",
    [
        span(status="ERROR"),
        span(amp_status=AmpSpanStatus(error=True)),
        span(attributes={"http.response.status_code": 446}),
        span(attributes={"http.status_code": "446"}),
        span(attributes={"http.response.status_code": 500}),
    ],
)
def test_root_failures(root):
    assert parse([root]).request_failed


@pytest.mark.parametrize("value", [200, "200", None, "invalid", True])
def test_unknown_or_successful_status_is_not_failure(value):
    trace = parse(
        [
            span(attributes={"http.response.status_code": value}),
            span("child", kind="llm", parent="root", status="ERROR"),
        ],
        error_count=1,
    )
    assert not trace.request_failed


@pytest.mark.parametrize("failed", [False, True])
def test_whole_trace_skip_does_not_call_any_evaluator(failed):
    @evaluator("trace-check")
    def trace_check(trace: Trace) -> EvalResult:
        pytest.fail("Excluded trace reached evaluator")

    @evaluator("agent-check")
    def agent_check(trace: AgentTrace) -> EvalResult:
        pytest.fail("Excluded trace reached evaluator")

    @evaluator("llm-check")
    def llm_check(trace: LLMSpan) -> EvalResult:
        pytest.fail("Excluded trace reached evaluator")

    trace = Trace(trace_id="t", spans=[AgentSpan(span_id="init", operation_name="create_agent")], request_failed=failed)
    result = Monitor(evaluators=[trace_check, agent_check, llm_check]).run(traces=[trace])
    for summary in result.scores.values():
        assert summary.count == 1
        assert summary.aggregated_scores == {}
        assert summary.skipped_count == 1
        score = summary.individual_scores[0]
        assert score.score is None
        assert score.skip_reason == ("Request failed" if failed else "Agent initialization")


def test_mixed_trace_scores_execution_and_preserves_initialization_context():
    calls = []

    @evaluator("agent-check")
    def agent_check(trace: AgentTrace) -> EvalResult:
        calls.append(trace.agent_id)
        return EvalResult(score=0.25)

    @evaluator("trace-check")
    def trace_check(trace: Trace) -> EvalResult:
        assert len(trace.get_agents()) == 2
        return EvalResult(score=0.5)

    @evaluator("llm-check")
    def llm_check(trace: LLMSpan) -> EvalResult:
        return EvalResult(score=0.75)

    trace = parse(
        [
            span(kind="chain"),
            span("init", operation="create_agent", parent="root"),
            span("run", operation="invoke_agent", parent="root"),
            span("llm", kind="llm", parent="run"),
        ]
    )
    result = Monitor(evaluators=[agent_check, trace_check, llm_check]).run(traces=[trace])
    assert calls == ["run"]
    summary = result.scores["agent-check"]
    assert summary.count == 2
    assert summary.skipped_count == 1
    assert summary.aggregated_scores["mean"] == 0.25
    assert summary.skipped_scores[0].span_context.span_id == "init"
    assert result.scores["trace-check"].count == 1
    assert result.scores["llm-check"].count == 1


def test_stream_continues_after_skips_and_preserves_recovered_errors():
    traces = [
        parse([span(operation="create_agent")]),
        parse([span(status="ERROR")]),
        parse([span(), span("child", kind="llm", parent="root", status="ERROR")], error_count=1),
    ]

    @evaluator("check")
    def check(trace: Trace) -> EvalResult:
        return EvalResult(score=0.25)

    result = Monitor(evaluators=[check]).run(traces=traces)
    summary = result.scores["check"]
    assert summary.count == 3
    assert summary.skipped_count == 2
    assert summary.aggregated_scores["mean"] == 0.25


@pytest.mark.parametrize("level", [Trace, AgentTrace, LLMSpan])
def test_initialization_skipped_in_experiment_and_direct_calls(level):
    def evaluate(value):
        pytest.fail("Initialization reached evaluator")

    evaluate.__annotations__ = {"value": level, "return": EvalResult}
    check = evaluator("check")(evaluate)
    trace = Trace(trace_id="t", spans=[AgentSpan(span_id="init", operation_name="create_agent")])
    assert check(trace)[0].skip_reason == "Agent initialization"
    assert check.run(trace)[0].skip_reason == "Agent initialization"
    result = Experiment(evaluators=[check], invoker=None).run(traces=[trace])
    summary = result.scores["check"]
    assert summary.skipped_count == 1
    assert summary.aggregated_scores == {}


def test_mixed_initialization_skipped_in_experiment_and_direct_calls():
    calls = []

    @evaluator("check")
    def check(trace: AgentTrace) -> EvalResult:
        calls.append(trace.agent_id)
        return EvalResult(score=0.5)

    trace = Trace(
        trace_id="t",
        spans=[
            AgentSpan(span_id="init", operation_name="create_agent"),
            AgentSpan(span_id="run", operation_name="invoke_agent"),
        ],
    )
    assert check(trace)[0].is_skipped
    result = Experiment(evaluators=[check], invoker=None).run(traces=[trace])
    assert calls == ["run", "run"]
    assert result.scores["check"].skipped_count == 1
    assert result.scores["check"].aggregated_scores["mean"] == 0.5


def test_explicit_initialization_opt_out():
    @evaluator("check")
    def check(trace: AgentTrace) -> EvalResult:
        return EvalResult(score=0.5)

    trace = Trace(
        trace_id="t", initialization_only=True, spans=[AgentSpan(span_id="init", operation_name="create_agent")]
    )
    assert check.run(trace, skip_initialization=False)[0].score == 0.5


def test_failed_request_skipped_in_every_mode():
    def evaluate(value):
        pytest.fail("Failed request reached evaluator")

    evaluate.__annotations__ = {"value": Trace, "return": EvalResult}
    check = evaluator("check")(evaluate)

    trace = Trace(trace_id="t", request_failed=True)
    assert check(trace)[0].skip_reason == "Request failed"
    assert check.run(trace)[0].skip_reason == "Request failed"
    assert Monitor(evaluators=[check]).run(traces=[trace]).scores["check"].skipped_count == 1
    experiment = Experiment(evaluators=[check], invoker=None).run(traces=[trace])
    assert experiment.scores["check"].skipped_count == 1
    assert experiment.scores["check"].aggregated_scores == {}


def test_explicit_failed_request_opt_out():
    @evaluator("check")
    def check(trace: Trace) -> EvalResult:
        return EvalResult(score=0.5)

    trace = Trace(trace_id="t", request_failed=True)
    assert check.run(trace, skip_failed_requests=False)[0].score == 0.5


def test_failed_request_takes_precedence_over_initialization():
    @evaluator("check")
    def check(trace: Trace) -> EvalResult:
        return EvalResult(score=0.5)

    trace = Trace(
        trace_id="t",
        request_failed=True,
        initialization_only=True,
        spans=[AgentSpan(span_id="init", operation_name="create_agent")],
    )
    assert check.run(trace)[0].skip_reason == "Request failed"


def test_fetched_traces_use_same_policy():
    class Fetcher:
        def fetch_traces(self, **kwargs):
            for operation in ["create_agent", "invoke_agent"]:
                root = span(operation=operation)
                yield OTELTrace(
                    traceId=operation,
                    rootSpanId="root",
                    rootSpanName=root.name,
                    startTime=root.startTime,
                    endTime=root.endTime,
                    spans=[root],
                )

    @evaluator("check")
    def check(trace: Trace) -> EvalResult:
        return EvalResult(score=0.5)

    result = Monitor(evaluators=[check], trace_fetcher=Fetcher()).run()
    assert result.scores["check"].count == 2
    assert result.scores["check"].skipped_count == 1


def test_explicit_empty_traces_do_not_fetch():
    @evaluator("check")
    def check(trace: Trace) -> EvalResult:
        pytest.fail("Empty trace list evaluated")

    assert Monitor(evaluators=[check]).run(traces=[]).traces_evaluated == 0


def test_missing_original_root_does_not_promote_child_failure():
    child = span("child", kind="llm", parent="missing", status="ERROR")
    assert not parse([child], error_count=1).request_failed


def test_traceloop_agent_semantics_are_execution():
    trace = parse(
        [span(name="weather.agent", attributes={"traceloop.span.kind": "agent", "traceloop.entity.name": "weather"})]
    )
    assert not trace.initialization_only
    assert trace.get_agents()[0].operation_name is None


def test_execution_nested_under_creation_is_preserved():
    trace = parse([span(operation="create_agent"), span("llm", kind="llm", parent="root")])
    assert not trace.initialization_only
    assert trace.get_llm_calls()[0].parent_span_id == "root"

    @evaluator("check")
    def check(trace: LLMSpan) -> EvalResult:
        return EvalResult(score=0.5)

    result = Monitor(evaluators=[check]).run(traces=[trace])
    assert result.scores["check"].count == 1
    assert result.scores["check"].skipped_count == 0
