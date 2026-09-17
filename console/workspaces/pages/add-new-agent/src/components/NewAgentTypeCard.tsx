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

import {
  Box,
  Typography,
  Form,
} from "@wso2/oxygen-ui";

interface NewAgentTypeCardProps {
  type: string;
  title: string;
  subheader: string;
  icon: React.ReactNode;
  onClick: (type: string) => void;
}

export const NewAgentTypeCard = (props: NewAgentTypeCardProps) => {
  const { type, title, subheader, icon, onClick } = props;
  const handleClick = () => {
    onClick(type);
  };

  return (
    <Form.CardButton
      onClick={handleClick}
      sx={{
        width: "100%",
        minHeight: 152,
        px: 3,
        py: 2,
        display: "flex",
        flexDirection: "row",
        alignItems: "center",
        justifyContent: "flex-start",
        gap: 3,
        textAlign: "left",
      }}
    >
      <Box
        sx={{
          height: 112,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
        }}
      >
        {icon}
      </Box>
      <Box display="flex" flexDirection="column" gap={1} alignItems="flex-start">
        <Typography variant="h3" textAlign="left">
          {title}
        </Typography>
        <Typography variant="body1" textAlign="left">
          {subheader}
        </Typography>
      </Box>
    </Form.CardButton>
  );
};
