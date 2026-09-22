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

package client

import "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"

// isTerminating reports whether a resource has been asked to go away but has not yet
// left the API.
//
// Deleting an OpenChoreo resource only sets metadata.deletionTimestamp: the object stays
// in the Kubernetes API, and therefore in every LIST response, until its cleanup finalizer
// (openchoreo.dev/project-cleanup and friends) has torn down the underlying builds,
// releases and workloads. That can take minutes. Listing such an object is wrong twice
// over: the console keeps rendering a resource the user just deleted, and the "is this
// still in use" guards (CountProjectComponents, ListComponentsByKind) refuse a delete on
// the strength of a dependent that is itself already on its way out.
//
// Every list in this package that returns user-visible resources or feeds one of those
// guards filters on this, so a delete reads as done the moment OpenChoreo accepts it.
func isTerminating(meta gen.ObjectMeta) bool {
	return meta.DeletionTimestamp != nil
}
