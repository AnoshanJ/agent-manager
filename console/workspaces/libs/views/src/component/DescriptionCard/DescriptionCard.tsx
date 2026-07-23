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

import { Card, CardContent, CardHeader, Divider, type SxProps, type Theme } from '@wso2/oxygen-ui';
import { MarkdownView } from '../MarkdownView';

export interface DescriptionCardProps {
  title: string;
  content: string;
  sx?: SxProps<Theme>;
}

/** An outlined card with a titled header rendering a markdown description below it. */
export function DescriptionCard({ title, content, sx }: DescriptionCardProps) {
  return (
    <Card variant="outlined" sx={sx}>
      <CardHeader title={title} />
      <Divider />
      <CardContent sx={{ '&:last-child': { pb: 2 } }}>
        <MarkdownView content={content} />
      </CardContent>
    </Card>
  );
}
