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
import { Sparkles } from "@wso2/oxygen-ui-icons-react";

interface NewAgentSampleCardProps {
  onClick: () => void;
}

export const NewAgentSampleCard = ({ onClick }: NewAgentSampleCardProps) => {
  return (
    <Form.CardButton
      onClick={onClick}
      selected
      sx={{
        width: "100%",
        height: "100%",
        px: 4,
        py: 4,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 2,
        textAlign: "center",
      }}
    >
      <Box
        sx={{
          height: 112,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Sparkles size={64} strokeWidth={1.5} />
      </Box>
      <Typography variant="h3" textAlign="center">
        Start quickly with a Sample
      </Typography>
      <Typography variant="body1" textAlign="center" sx={{ maxWidth: 320 }}>
        Launch a ready-made agent from the catalog and customize it from there.
      </Typography>
    </Form.CardButton>
  );
};
