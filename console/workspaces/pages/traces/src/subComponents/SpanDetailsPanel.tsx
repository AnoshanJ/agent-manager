/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Chip, Stack, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import {
  Span,
  LLMData,
  AgentData,
  ToolDefinition,
  ToolData,
  CrewAITaskData,
  EvaluatorScoreWithMonitor,
} from "@agent-management-platform/types";
import { BasicInfoSection } from "./spanDetails/BasicInfoSection";
import { AttributesSection } from "./spanDetails/AttributesSection";
import { useEffect, useState } from "react";
import { ToolsSection } from "./spanDetails/ToolsSection";
import { FadeIn } from "@agent-management-platform/views";
import { Overview } from "./spanDetails/Overview";
import { ScoresSection } from "./spanDetails/ScoresSection";

interface SpanDetailsPanelProps {
  span: Span | null;
  evaluatorScores?: EvaluatorScoreWithMonitor[];
}

// Helper function to extract tools from data based on span kind
function getTools(span: Span): ToolDefinition[] | string[] | undefined {
  const { kind, data } = span.ampAttributes || {};
  if (kind === "llm" && data) {
    return (data as LLMData).tools;
  } else if (kind === "agent" && data) {
    return (data as AgentData).tools;
  }
  return undefined;
}

// Converts a Python dict literal into JSON text.
function pythonDictToJson(input: string): string {
  const ESCAPES: Record<string, string> = {
    n: "\n",
    t: "\t",
    r: "\r",
    "\\": "\\",
    "'": "'",
    '"': '"',
  };
  let out = "";
  let buf = ""; 
  const flush = () => {
    out += buf
      .replace(/\bNone\b/g, "null")
      .replace(/\bTrue\b/g, "true")
      .replace(/\bFalse\b/g, "false");
    buf = "";
  };
  let i = 0;
  while (i < input.length) {
    const ch = input[i];
    if (ch === "'" || ch === '"') {
      flush();
      const quote = ch;
      i++;
      let str = "";
      while (i < input.length) {
        const c = input[i];
        if (c === "\\") {
          const next = input[i + 1] ?? "";
          str += next in ESCAPES ? ESCAPES[next] : next;
          i += 2;
          continue;
        }
        if (c === quote) {
          i++;
          break;
        }
        str += c;
        i++;
      }
      out += JSON.stringify(str); // re-encode with proper JSON escaping
      continue;
    }
    buf += ch;
    i++;
  }
  flush();
  return out;
}

function parseEmbeddedObject(message: string): Record<string, unknown> | undefined {
  const start = message.indexOf("{");
  const end = message.lastIndexOf("}");
  if (start === -1 || end === -1 || end <= start) {
    return undefined;
  }
  const candidate = message.slice(start, end + 1);

  for (const text of [candidate, pythonDictToJson(candidate)]) {
    try {
      const parsed = JSON.parse(text);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>;
      }
    } catch {
      // try the next candidate
    }
  }
  return undefined;
}

// build the span status content rendered in the Status Message tab.
function getStatusDetails(span: Span): Record<string, unknown> | string | undefined {
  const status = span.ampAttributes?.status;
  if (!status?.message) {
    return undefined;
  }
  return parseEmbeddedObject(status.message) ?? status.message;
}

// Helper function to check if span has overview content
function hasOverviewContent(span: Span): boolean {
  const { kind, data, input, output } = span.ampAttributes || {};

  // Check for input/output
  if (input || output) {
    return true;
  }

  // Check for agent name or system prompt
  if (kind === "agent" && data) {
    const agentData = data as AgentData;
    if (agentData.name || agentData.systemPrompt) {
      return true;
    }
  }

  // Check for tool name
  if (kind === "tool" && data) {
    const toolData = data as ToolData;
    if (toolData.name) {
      return true;
    }
  }

  // Check for CrewAI task name or description
  if (kind === "crewaitask" && data) {
    const taskData = data as CrewAITaskData;
    if (taskData.name || taskData.description) {
      return true;
    }
  }

  return false;
}

export function SpanDetailsPanel({ span, evaluatorScores }: SpanDetailsPanelProps) {
  const [selectedTab, setSelectedTab] = useState<string>("overview");
  const hasOverview = span ? hasOverviewContent(span) : false;
  const tools = span ? getTools(span) : undefined;
  const hasTools = !!tools;
  const hasAttributes = !!span?.attributes;
  const hasScores = !!(evaluatorScores && evaluatorScores.length > 0);
  const statusDetails = span ? getStatusDetails(span) : undefined;
  const hasStatus = !!statusDetails;

  useEffect(() => {
    if (!span) return;

    if (selectedTab === "overview" && hasOverview) {
      return;
    } else if (selectedTab === "tools" && hasTools) {
      return;
    } else if (selectedTab === "attributes" && hasAttributes) {
      return;
    } else if (selectedTab === "status" && hasStatus) {
      return;
    } else if (selectedTab === "scores" && hasScores) {
      return;
    }
    // Check if there's overview content (input/output/name/systemPrompt)
    if (hasOverview) {
      setSelectedTab("overview");
    }
    // for tools
    else if (hasTools) {
      setSelectedTab("tools");
    }
    // for attributes
    else if (hasAttributes) {
      setSelectedTab("attributes");
    }
    // for status message
    else if (hasStatus) {
      setSelectedTab("status");
    }
    // for scores
    else if (hasScores) {
      setSelectedTab("scores");
    }
  }, [span, span?.spanId, hasOverview, hasTools, hasAttributes, hasStatus, hasScores, selectedTab]);

  if (!span) {
    return null;
  }

  return (
    <Stack sx={{ height: "100%" }}>
      <Stack spacing={2}>
        <Stack spacing={2} px={1}>
          <Stack direction="row" spacing={1}>
            <Typography
              variant="h4"
              noWrap
              textOverflow="ellipsis"
              maxWidth="70%"
              overflow="hidden"
            >
              {span.name}
            </Typography>{" "}
            {span.ampAttributes?.kind && (
              <Chip
                size="small"
                variant="outlined"
                label={span.ampAttributes?.kind.toUpperCase()}
              />
            )}
          </Stack>
          <BasicInfoSection span={span} evaluatorScores={evaluatorScores} />
        </Stack>
        <Tabs
          variant="fullWidth"
          value={selectedTab}
          onChange={(_event, newValue) => setSelectedTab(newValue)}
        >
          <Tab label="Overview" value="overview" disabled={!hasOverview} />
          {hasTools && <Tab label="Tools" value="tools" />}
          {span?.attributes && <Tab label="Attributes" value="attributes" />}
          {hasStatus && <Tab label="Status Message" value="status" />}
          {hasScores && <Tab label="Scores" value="scores" />}
        </Tabs>
      </Stack>
      <Stack
        spacing={2}
        px={1}
        sx={{
          overflowY: "auto",
          flexGrow: 1,
          boxShadow: "inset 0 4px 8px -4px rgba(0, 0, 0, 0.1)",
        }}
      >
        {selectedTab === "attributes" && (
          <FadeIn>
            <AttributesSection attributes={span?.attributes} />
          </FadeIn>
        )}
        {selectedTab === "status" && statusDetails && (
          <FadeIn>
            <AttributesSection attributes={statusDetails} />
          </FadeIn>
        )}
        {selectedTab === "tools" && (
          <FadeIn>
            <ToolsSection tools={tools || []} />
          </FadeIn>
        )}
        {selectedTab === "overview" && (
          <FadeIn>
            <Overview ampAttributes={span.ampAttributes} />
          </FadeIn>
        )}
        {selectedTab === "scores" && hasScores && (
          <FadeIn>
            <ScoresSection evaluatorScores={evaluatorScores!} />
          </FadeIn>
        )}
      </Stack>
    </Stack>
  );
}
