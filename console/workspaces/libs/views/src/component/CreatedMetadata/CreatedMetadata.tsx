/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { Box, Typography } from '@wso2/oxygen-ui';
import { Clock } from '@wso2/oxygen-ui-icons-react';
import { formatDistanceToNow } from 'date-fns';

export interface CreatedMetadataProps {
  createdAt?: string;
}

/** Renders "Created ... ago" from an ISO timestamp; used as a page subheader on overview pages. */
export function CreatedMetadata({ createdAt }: CreatedMetadataProps) {
  if (!createdAt) return null;

  return (
    <Box display="flex" alignItems="center" gap={0.5}>
      <Clock size={12} />
      <Typography variant="caption" color="text.secondary">Created:</Typography>
      <Typography variant="caption" fontWeight={500}>
        {formatDistanceToNow(new Date(createdAt), { addSuffix: true })}
      </Typography>
    </Box>
  );
}
