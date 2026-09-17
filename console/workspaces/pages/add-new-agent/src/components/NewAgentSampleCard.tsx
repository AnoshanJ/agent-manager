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

import { Box, Typography, Form } from "@wso2/oxygen-ui";
import { sampleAgents } from "../data/sampleAgents";

interface NewAgentSampleCardProps {
  onSelectSample: (sampleId: string) => void;
}

export const NewAgentSampleCard = ({ onSelectSample }: NewAgentSampleCardProps) => {
  return (
    <Form.Section>
      <Typography variant="h3">Start Quickly with a Sample</Typography>
      <Typography variant="body2" color="text.secondary">
        Choose a sample agent to configure and deploy as a Platform-Hosted Agent.
      </Typography>
      <Form.Stack spacing={1.5}>
        {sampleAgents.map((sample) => (
          <Form.CardButton
            key={sample.id}
            onClick={() => onSelectSample(sample.id)}
            sx={{
              px: 2,
              py: 1.5,
              display: "flex",
              flexDirection: "row",
              alignItems: "center",
              justifyContent: "flex-start",
              gap: 2,
              textAlign: "left",
            }}
          >
            <Box
              sx={{
                height: 40,
                width: 40,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                flexShrink: 0,
              }}
            >
              <sample.icon size={24} strokeWidth={1.5} />
            </Box>
            <Box display="flex" flexDirection="column" alignItems="flex-start">
              <Typography variant="body1" fontWeight={600} textAlign="left">
                {sample.title}
              </Typography>
              <Typography variant="body2" color="text.secondary" textAlign="left">
                {sample.description}
              </Typography>
              <Typography variant="caption" color="text.secondary" textAlign="left">
                Requires {sample.requirements.join(" • ")}
              </Typography>
            </Box>
          </Form.CardButton>
        ))}
      </Form.Stack>
    </Form.Section>
  );
};
