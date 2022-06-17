package util

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/graph"
)

// badServiceMatcher looks for a physical IP address with optional port (e.g. 10.11.12.13:80)
var badServiceMatcher = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+(:\d+)?$`)
var egressHost string

// HandleDestination modifies the destination information, when necessary, for various corner
// cases.  It should be called after source validation and before destination processing.
// Returns destSvcNs, destSvcName, destWlNs, destWl, destApp, destVersion, isupdated
func HandleDestination(sourceWlNs, sourceWl, destSvcNs, destSvc, destSvcName, destWlNs, destWl, destApp, destVer string) (string, string, string, string, string, string, bool) {
	if destSvcNs, destSvcName, isUpdated := handleMultiClusterRequest(sourceWlNs, sourceWl, destSvcNs, destSvcName); isUpdated {
		return destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, true
	}

	// Handle egressgateway (kiali#2999)
	if egressHost == "" {
		egressHost = fmt.Sprintf("istio-egressgateway.%s.svc.cluster.local", config.Get().IstioNamespace)
	}

	if destSvc == egressHost && destSvc == destSvcName {
		istioNs := config.Get().IstioNamespace
		return istioNs, "istio-egressgateway", istioNs, "istio-egressgateway", "istio-egressgateway", "latest", true
	}

	return destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, false
}

// handleMultiClusterRequest 确保适当的目的地服务命名空间和名称请求转发来自另一个集群(通过ServiceEntry)。
// handleMultiClusterRequest ensures the proper destination service namespace and name
// for requests forwarded from another cluster (via a ServiceEntry).

// 给定一个请求从集群、集群B,集群A将生成source telemetry(从源工作负载到ServiceEntry)，集群B将生成destination telemetry(从unknown到目的地的工作负载)。
// 如果是一个destination telemetry，destination_service_name的标签将会被设置成service entry的host，它需要具有 <name>.<namespace>.global 格式，其中 name 和 namespace 分别对应于远程服务的名称和命名空间。
//在这种情况下我们改变请求有两种方式:
// 首先，我们将 destSvcName 重置为 <name>，以便统一对服务的远程和本地请求。 通过这样做，graph将仅显示一个 <service> 节点，而不是 <service> 和 <name>.<namespace>.global 这两个节点在实践中是相同的。

// 其次，我们将 destSvcNs 重置为 <namespace>。 我们希望将 destSvcNs 设置为远程服务命名空间的命名空间。 但在实践中，它将被设置为定义servieEntry 的命名空间（在clusterA 上）。 这对可视化没有用，所以我们在这里替换它。
//请注意，<namespace> 应该等同于为destination_workload_namespace 设置的值，为了方便，我们只是使用<namespace>，我们在这里。

// 所有这一切仅在源工作负载为“unknown”的情况下完成，这表明这表示 clusterB 上的destination telemetry，并且 destSvcName 为 MC 格式。 当源工作负载已知流量时，它应该代表集群流量到 ServiceEntry 并被路由出集群。 该用例在 service_entry.go 文件中处理。

// Given a request from clusterA to clusterB, clusterA will generate source telemetry
// (from the source workload to the service entry) and clusterB will generate destination
// telemetry (from unknown to the destination workload). If this is the destination
// telemetry the destination_service_name label will be set to the service entry host,
// which is required to have the form <name>.<namespace>.global where name and namespace
// correspond to the remote service’s name and namespace respectively. In this situation
// we alter the request in two ways:
//
// First, we reset destSvcName to <name> in order to unify remote and local requests to the
// service. By doing this the graph will show only one <service> node instead of having a
// node for both <service> and <name>.<namespace>.global which in practice, are the same.
//
// Second, we reset destSvcNs to <namespace>. We want destSvcNs to be set to the namespace
// of the remote service's namespace.  But in practice it will be set to the namespace
// (on clusterA) where the servieEntry is defined. This is not useful for the visualization,
// and so we replace it here. Note that <namespace> should be equivalent to the value set for
// destination_workload_namespace, we just use <namespace> for convenience, we have it here.
//
// All of this is only done if source workload is "unknown", which is what indicates that
// this represents the destination telemetry on clusterB, and if the destSvcName is in
// the MC format. When the source workload IS known the traffic it should be representing
// the clusterA traffic to the ServiceEntry and being routed out of the cluster. That use
// case is handled in the service_entry.go file.
//
// Returns destSvcNs, destSvcName, isUpdated
func handleMultiClusterRequest(sourceWlNs, sourceWl, destSvcNs, destSvcName string) (string, string, bool) {
	if sourceWlNs == graph.Unknown && sourceWl == graph.Unknown {
		destSvcNameEntries := strings.Split(destSvcName, ".")

		if len(destSvcNameEntries) == 3 && destSvcNameEntries[2] == config.IstioMultiClusterHostSuffix {
			return destSvcNameEntries[1], destSvcNameEntries[0], true
		}
	}

	return destSvcNs, destSvcName, false
}

// HandleResponseCode returns either the HTTP response code or the GRPC response status.  GRPC response
// status was added upstream in Istio 1.5 and downstream OSSM 1.1.  We support it here in a backward compatible
// way.  When protocol is not GRPC, or if the version running does not supply the GRPC status, just return the
// HTTP code.  Also return the HTTP code In the rare case that protocol is GRPC but the HTTP transport fails. (I
// have never seen this happen).  Otherwise, return the GRPC status.
func HandleResponseCode(protocol, httpResponseCode string, grpcResponseStatusOk bool, grpcResponseStatus string) string {
	if protocol != graph.GRPC.Name || graph.IsHTTPErr(httpResponseCode) || !grpcResponseStatusOk {
		return httpResponseCode
	}

	return grpcResponseStatus
}

// IsBadSourceTelemetry tests for known issues in generated telemetry given indicative label values.
// 1) source namespace is provided with neither workload nor app
// 2) no more conditions known
func IsBadSourceTelemetry(ns, wl, app string) bool {
	// case1
	return graph.IsOK(ns) && !graph.IsOK(wl) && !graph.IsOK(app)
}

// IsBadDestTelemetry tests for known issues in generated telemetry given indicative label values.
// 1) During pod lifecycle changes incomplete telemetry may be generated that results in
//    destSvc == destSvcName and no dest workload, where destSvc[Name] is in the form of an IP address.
// 2) no more conditions known
func IsBadDestTelemetry(svc, svcName, wl string) bool {
	// case1
	failsEqualsTest := (!graph.IsOK(wl) && graph.IsOK(svc) && graph.IsOK(svcName) && (svc == svcName))
	return failsEqualsTest && badServiceMatcher.MatchString(svcName)
}
