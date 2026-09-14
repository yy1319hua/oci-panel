package controllers

import (
	"encoding/json"
	"testing"

	"github.com/adiecho/oci-panel/internal/models"
)

// TestBuildBotInstancesKeepsPublicIPs 是本仓库一处真实线上问题的回归测试。
//
// 【背景】机器人收到「oci 状态」后，明明实例有公网 IP，却显示「无公网IP」，
// 而同一实例在面板里能正常显示。根因是 bot 摘要此前直接使用不含 VNIC 信息的
// 精简实例列表：InstanceInfo 的 PublicIPs 只在查询 VNIC 详情时才被填充，
// 精简列表里该字段恒为零值。
//
// 这个测试锁定「公网 IP / IPv6 必须原样带到 bot 输出」这一契约：
// 若将来有人再把数据源换成不含 VNIC 的列表，这里会立刻失败。
func TestBuildBotInstancesKeepsPublicIPs(t *testing.T) {
	in := []models.InstanceInfo{
		{
			ID:          "ocid1.instance.oc1..amd1",
			DisplayName: "amd1",
			State:       "RUNNING",
			Shape:       "VM.Standard.E2.1.Micro",
			PublicIPs:   []string{"203.0.113.10"},
			IPv6:        "2603:c020:8005::10",
		},
		{
			ID:          "ocid1.instance.oc1..amd2",
			DisplayName: "amd2",
			State:       "RUNNING",
			Shape:       "VM.Standard.E2.1.Micro",
			PublicIPs:   []string{"203.0.113.11"},
		},
		{
			// 真正没有公网 IP 的实例，应保持为空（不能被误填）。
			ID:          "ocid1.instance.oc1..arm",
			DisplayName: "arm",
			State:       "STOPPED",
			Shape:       "VM.Standard.A1.Flex",
			PublicIPs:   []string{},
		},
	}

	out := buildBotInstances(in)

	if len(out) != 3 {
		t.Fatalf("实例数 = %d, want 3", len(out))
	}

	if len(out[0].PublicIPs) != 1 || out[0].PublicIPs[0] != "203.0.113.10" {
		t.Fatalf("第 1 台公网 IP 丢失或错误：%v", out[0].PublicIPs)
	}
	if out[0].IPv6 != "2603:c020:8005::10" {
		t.Fatalf("第 1 台 IPv6 丢失：%q", out[0].IPv6)
	}
	if out[0].Name != "amd1" || out[0].State != "RUNNING" || out[0].Shape != "VM.Standard.E2.1.Micro" {
		t.Fatalf("第 1 台基本字段错误：%+v", out[0])
	}

	if len(out[1].PublicIPs) != 1 || out[1].PublicIPs[0] != "203.0.113.11" {
		t.Fatalf("第 2 台公网 IP 丢失或错误：%v", out[1].PublicIPs)
	}

	if len(out[2].PublicIPs) != 0 {
		t.Fatalf("第 3 台本无公网 IP，却被填充：%v", out[2].PublicIPs)
	}
}

// TestBuildBotInstancesJSONShape 锁定对外 JSON 字段名。
// 脚本与第三方调用方都是按 publicIps / ipv6 取值的，字段名变了会直接让显示回归。
func TestBuildBotInstancesJSONShape(t *testing.T) {
	out := buildBotInstances([]models.InstanceInfo{
		{ID: "i-1", DisplayName: "amd1", State: "RUNNING", PublicIPs: []string{"203.0.113.10"}, IPv6: "::1"},
	})

	raw, err := json.Marshal(out[0])
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("反序列化失败：%v", err)
	}

	for _, key := range []string{"id", "name", "state", "shape", "publicIps", "ipv6"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("缺少 JSON 字段 %q，实际字段：%v", key, m)
		}
	}
}

// TestBuildBotInstancesEmptyInput 空输入应返回空切片（而非 nil），
// 保证 JSON 序列化成 [] 而不是 null，调用方无需额外判空。
func TestBuildBotInstancesEmptyInput(t *testing.T) {
	out := buildBotInstances(nil)
	if out == nil {
		t.Fatal("空输入应返回空切片而非 nil")
	}
	if len(out) != 0 {
		t.Fatalf("空输入应返回 0 个实例，实际 %d", len(out))
	}
	raw, _ := json.Marshal(out)
	if string(raw) != "[]" {
		t.Fatalf("空实例应序列化为 []，实际 %s", raw)
	}
}
