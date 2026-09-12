package services

import "testing"

// TestIsManagedNlbForInstance 是针对「Disable/Enable500Mbps 误删整 compartment 负载均衡器」
// 这一数据丢失级 bug 的回归测试：只有由 oci-panel 为「同一实例」创建（带受管标签）的 NLB
// 才允许被删除，其他实例的受管 NLB 与用户自建 NLB 都不得匹配。
func TestIsManagedNlbForInstance(t *testing.T) {
	const inst = "ocid1.instance.oc1..aaaaaaaa"
	tests := []struct {
		name       string
		tags       map[string]string
		instanceID string
		want       bool
	}{
		{"managed for this instance", map[string]string{nlbManagedTagKey: nlbManagedTagValue, nlbInstanceTagKey: inst}, inst, true},
		{"managed but different instance", map[string]string{nlbManagedTagKey: nlbManagedTagValue, nlbInstanceTagKey: "ocid1.instance.oc1..bbbbbbbb"}, inst, false},
		{"user-created NLB (no managed tag)", map[string]string{"team": "prod"}, inst, false},
		{"nil tags", nil, inst, false},
		{"managed flag not true", map[string]string{nlbManagedTagKey: "false", nlbInstanceTagKey: inst}, inst, false},
		{"managed without instance tag", map[string]string{nlbManagedTagKey: nlbManagedTagValue}, inst, false},
		{"empty instanceID never matches", map[string]string{nlbManagedTagKey: nlbManagedTagValue, nlbInstanceTagKey: ""}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isManagedNlbForInstance(tt.tags, tt.instanceID); got != tt.want {
				t.Errorf("isManagedNlbForInstance(%v, %q) = %v, want %v", tt.tags, tt.instanceID, got, tt.want)
			}
		})
	}
}

// TestManagedNlbFreeformTags 验证创建时打的标签与删除时的判定函数互相自洽：
// 同一实例认得，跨实例不认。
func TestManagedNlbFreeformTags(t *testing.T) {
	const inst = "ocid1.instance.oc1..cccccccc"
	tags := managedNlbFreeformTags(inst)

	if tags[nlbManagedTagKey] != nlbManagedTagValue {
		t.Errorf("managed tag = %q, want %q", tags[nlbManagedTagKey], nlbManagedTagValue)
	}
	if tags[nlbInstanceTagKey] != inst {
		t.Errorf("instance tag = %q, want %q", tags[nlbInstanceTagKey], inst)
	}
	if !isManagedNlbForInstance(tags, inst) {
		t.Error("tags from managedNlbFreeformTags must be recognized as managed for the same instance")
	}
	if isManagedNlbForInstance(tags, "ocid1.instance.oc1..dddddddd") {
		t.Error("tags for one instance must not match a different instance")
	}
}
