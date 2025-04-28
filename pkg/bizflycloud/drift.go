package bizflycloud

import (
	"context"

	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// Define metrics for drift detection
const ()

func (c *CloudProvider) isNodeClassDrifted(
	ctx context.Context,
	nodeClaim *v1.NodeClaim,
	nodePool *v1.NodePool,
	nodeClass *v1bizfly.BizflyCloudNodeClass,
) (cloudprovider.DriftReason, error) {
	// First check if the node class is statically drifted to save on API calls.
	// if drifted := c.areStaticFieldsDrifted(nodeClaim, nodeClass); drifted != "" {
	// 	return drifted, nil
	// }
	// instance, err := c.getInstance(ctx, nodeClaim.Status.ProviderID)
	// if err != nil {
	// 	return "", err
	// }
	// amiDrifted, err := c.isAMIDrifted(ctx, nodeClaim, nodePool, instance, nodeClass)
	// if err != nil {
	// 	return "", fmt.Errorf("calculating ami drift, %w", err)
	// }
	// securitygroupDrifted, err := c.areSecurityGroupsDrifted(instance, nodeClass)
	// if err != nil {
	// 	return "", fmt.Errorf("calculating securitygroup drift, %w", err)
	// }
	// subnetDrifted, err := c.isSubnetDrifted(instance, nodeClass)
	// if err != nil {
	// 	return "", fmt.Errorf("calculating subnet drift, %w", err)
	// }
	// capacityReservationsDrifted := c.isCapacityReservationDrifted(instance, nodeClass)
	// drifted := lo.FindOrElse([]cloudprovider.DriftReason{
	// 	amiDrifted,
	// 	securitygroupDrifted,
	// 	subnetDrifted,
	// 	capacityReservationsDrifted,
	// }, "", func(i cloudprovider.DriftReason) bool {
	// 	return string(i) != ""
	// })
	// return drifted, nil
	return "", nil
}
