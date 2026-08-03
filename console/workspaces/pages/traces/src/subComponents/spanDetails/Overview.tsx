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

import { NoDataFound, JSONView, MarkdownView } from "@agent-management-platform/views";
import {
  Box,
  Card,
  CardContent,
  Chip,
  Divider,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Info } from "@wso2/oxygen-ui-icons-react";
import {
  AmpAttributes,
  PromptMessage,
  ToolData,
  AgentData,
  CrewAITaskData,
} from "@agent-management-platform/types";
import { memo, useCallback, useMemo } from "react";

interface OverviewProps {
  ampAttributes?: AmpAttributes;
}

interface MessageListProps {
  title: string;
  messages: Partial<PromptMessage>[];
  getRoleColor: (role: string) => "default" | "primary" | "success" | "info";
  "data-testid"?: string;
  showEmptyMessage?: boolean;
}

function formattedMessage(message: string) {
  /**
   * Recursively parse JSON strings, including nested JSON strings
   * within the parsed object/array
   */
  function recursiveParse(value: any): any {
    // If it's a string, try to parse it as JSON
    if (typeof value === "string") {
      try {
        const parsed = JSON.parse(value);
        // Recursively process the parsed result
        return recursiveParse(parsed);
      } catch {
        // If parsing fails, return the original string
        return value;
      }
    }

    // If it's an array, recursively process each element
    if (Array.isArray(value)) {
      return value.map((item) => recursiveParse(item));
    }

    // If it's an object, recursively process each property
    if (value !== null && typeof value === "object") {
      const result: Record<string, any> = {};
      for (const key in value) {
        if (Object.prototype.hasOwnProperty.call(value, key)) {
          result[key] = recursiveParse(value[key]);
        }
      }
      return result;
    }

    // For primitives (number, boolean, null), return as-is
    return value;
  }

  try {
    const parsed = recursiveParse(message);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return message;
  }
}

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
  let buf = ""; // pending non-string run
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

// Extracts and parses the object embedded in a status message such as
// "Error code: 422 - {'message': {...}, 'type': '...'}". Scans for the balanced
// object (ignoring braces inside strings), then tries plain JSON first and a
// Python-dict normalization. Returns the parsed object, or undefined when nothing
// parseable is present.
function parseEmbeddedObject(message: string): Record<string, unknown> | undefined {
  const start = message.indexOf("{");
  if (start === -1) {
    return undefined;
  }
  let depth = 0;
  let quote: string | null = null;
  let end = -1;
  for (let i = start; i < message.length; i++) {
    const ch = message[i];
    if (quote) {
      if (ch === "\\") { i++; continue; }
      if (ch === quote) quote = null;
      continue;
    }
    if (ch === "'" || ch === '"') { quote = ch; continue; }
    if (ch === "{") depth++;
    else if (ch === "}") {
      depth--;
      if (depth === 0) { end = i; break; }
    }
  }
  if (end === -1) {
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

// Builds the status-message content shown in the Overview tab. When the message
// wraps an embedded payload (e.g. "Error code: 422 - {...}"), the prefix is
// dropped and the payload is pretty-printed as JSON; otherwise the raw message
// is shown as-is.
function formatStatusMessage(message: string): string {
  const parsed = parseEmbeddedObject(message);
  return parsed ? formattedMessage(JSON.stringify(parsed)) : message;
}
const MessageList = memo(function MessageList({
  title,
  messages,
  getRoleColor,
  "data-testid": testId,
  showEmptyMessage = false,
}: MessageListProps) {
  if (messages.length === 0) {
    if (!showEmptyMessage) {
      return null;
    }

    return (
      <Box data-testid={testId}>
        <Typography variant="h6" sx={{ mb: 2 }}>
          {title}
        </Typography>
        <Card variant="outlined" sx={{ bgcolor: "action.hover" }}>
          <CardContent>
            <Typography variant="body2" color="text.secondary">
              No data available
            </Typography>
          </CardContent>
        </Card>
      </Box>
    );
  }

  return (
    <Stack pt={2} data-testid={testId}>
      <Divider sx={{ mb: 2 }} />
      <Typography variant="h6" sx={{ mb: 2 }}>
        {title}
      </Typography>
      <Stack spacing={2}>
        {messages.map((message, index) => {
          const messageKey =
            (message as PromptMessage & { id?: string }).id ?? index;
          return (
            <Card key={messageKey} variant="outlined">
              <CardContent>
                <Stack spacing={1.5}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    {message?.role && message.role !== "unknown" && (
                      <Chip
                        label={
                          message.role.charAt(0).toUpperCase() +
                          message.role.slice(1)
                        }
                        size="small"
                        color={getRoleColor(message.role)}
                        variant="outlined"
                      />
                    )}
                  </Box>
                  {message.content && !message.role && (
                    <JSONView json={formattedMessage(message.content)} />
                  )}
                  {message.content && message.role && (
                    <MarkdownView content={message.content} />
                  )}
                  {message.toolCalls && message.toolCalls.length > 0 && (
                    <Box>
                      <Stack spacing={1}>
                        {message.toolCalls.map((toolCall, toolIndex) => {
                          const toolCallKey = toolCall.id ?? toolIndex;

                          return (
                            <Card key={toolCallKey} variant="outlined">
                              <CardContent sx={{ "&:last-child": { pb: 1.5 } }}>
                                <Typography
                                  variant="caption"
                                  sx={{ fontWeight: "bold" }}
                                >
                                  {toolCall.name}
                                </Typography>
                                {toolCall.arguments && (
                                  <Typography
                                    variant="caption"
                                    sx={{
                                      display: "block",
                                      mt: 0.5,
                                      fontFamily: "monospace",
                                      whiteSpace: "pre-wrap",
                                      wordBreak: "break-word",
                                    }}
                                  >
                                    {formattedMessage(toolCall.arguments)}
                                  </Typography>
                                )}
                              </CardContent>
                            </Card>
                          );
                        })}
                      </Stack>
                    </Box>
                  )}
                </Stack>
              </CardContent>
            </Card>
          );
        })}
      </Stack>
    </Stack>
  );
});

export function Overview({ ampAttributes }: OverviewProps) {
  const normalizeMessages = useCallback(
    (
      input: PromptMessage[] | string[] | string | undefined
    ): (Partial<PromptMessage> | { content: string })[] => {
      if (!input) return [];
      if (typeof input === "string") {
        return [{ content: input }];
      }
      // Handle string arrays (e.g., for embedding documents)
      if (
        Array.isArray(input) &&
        input.length > 0 &&
        typeof input[0] === "string"
      ) {
        return (input as string[]).map((doc) => ({ content: doc }));
      }
      // Handle PromptMessage arrays
      return input as PromptMessage[];
    },
    []
  );

  const inputMessages = useMemo(
    () => normalizeMessages(ampAttributes?.input),
    [ampAttributes?.input, normalizeMessages]
  );

  const outputMessages = useMemo(
    () => normalizeMessages(ampAttributes?.output),
    [ampAttributes?.output, normalizeMessages]
  );

  // Extract name from data based on kind
  const name = useMemo(() => {
    const { kind, data } = ampAttributes || {};
    if (kind === "tool" && data) {
      return (data as ToolData).name;
    } else if (kind === "agent" && data) {
      return (data as AgentData).name;
    } else if (kind === "crewaitask" && data) {
      return (data as CrewAITaskData).name;
    }
    return undefined;
  }, [ampAttributes]);

  // Extract description for CrewAI tasks
  const taskDescription = useMemo(() => {
    const { kind, data } = ampAttributes || {};
    if (kind === "crewaitask" && data) {
      return (data as CrewAITaskData).description;
    }
    return undefined;
  }, [ampAttributes]);

  // Extract system prompt for agent spans
  const systemPrompt = useMemo(() => {
    const { kind, data } = ampAttributes || {};
    if (kind === "agent" && data) {
      return (data as AgentData).systemPrompt;
    }
    return undefined;
  }, [ampAttributes]);

  // Status message (e.g. an error/guardrail response) shown as JSON, like the
  // input/output message content.
  const statusMessage = useMemo(() => {
    const raw = ampAttributes?.status?.message;
    return raw ? formatStatusMessage(raw) : undefined;
  }, [ampAttributes?.status?.message]);

  const hasContent = inputMessages.length > 0 || outputMessages.length > 0;

  const getRoleColor = useCallback((role: string) => {
    switch (role) {
      case "system":
        return "default";
      case "user":
        return "primary";
      case "assistant":
        return "success";
      case "tool":
        return "info";
      default:
        return "default";
    }
  }, []);

  if (!hasContent && !name && !statusMessage) {
    return (
      <NoDataFound
        message="Failed to extract span details"
        iconElement={Info}
        subtitle="Try selecting a different span"
        disableBackground
      />
    );
  }

  return (
    <Stack spacing={3}>
      {name && (
        <Stack>
          <Typography variant="h6">Name</Typography>
          <Card variant="outlined">
            <CardContent>
              <Typography variant="body2">{name}</Typography>
            </CardContent>
          </Card>
        </Stack>
      )}

      {taskDescription && (
        <Stack>
          <Typography variant="h6">Description</Typography>
          <Card variant="outlined">
            <CardContent>
              <Typography
                variant="body2"
                sx={{
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-word",
                }}
              >
                {taskDescription}
              </Typography>
            </CardContent>
          </Card>
        </Stack>
      )}

      {systemPrompt && (
        <Stack>
          <Typography variant="h6">System Prompt</Typography>
          <Card variant="outlined">
            <CardContent>
              <Typography
                variant="body2"
                sx={{
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-word",
                }}
              >
                {formattedMessage(systemPrompt)}
              </Typography>
            </CardContent>
          </Card>
        </Stack>
      )}

      <MessageList
        title="Input Messages"
        messages={inputMessages}
        getRoleColor={getRoleColor}
        data-testid="input-messages"
        showEmptyMessage={false}
      />
      <MessageList
        title="Output Messages"
        messages={outputMessages}
        getRoleColor={getRoleColor}
        data-testid="output-messages"
        showEmptyMessage={false}
      />

      {statusMessage && (
        <Stack pt={2} data-testid="status-message">
          <Divider sx={{ mb: 2 }} />
          <Typography variant="h6" sx={{ mb: 2 }}>
            Status Message
          </Typography>
          <Card variant="outlined">
            <CardContent>
              <JSONView json={statusMessage} />
            </CardContent>
          </Card>
        </Stack>
      )}
    </Stack>
  );
}
