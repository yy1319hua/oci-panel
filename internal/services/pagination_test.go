package services

import (
	"errors"
	"testing"
)

// TestPaginateSinglePage 验证 B4：无下一页时只调用一次并返回全部条目。
func TestPaginateSinglePage(t *testing.T) {
	calls := 0
	got, err := paginate(func(page *string) ([]int, *string, error) {
		calls++
		if page != nil {
			t.Errorf("first page token = %q, want nil", *page)
		}
		return []int{1, 2, 3}, nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
}

// TestPaginateMultiPage 验证 B4：跨多页累积，且下一页 token 被正确回传。
func TestPaginateMultiPage(t *testing.T) {
	pages := [][]int{{1, 2}, {3, 4}, {5}}
	tokens := []string{"p1", "p2"} // page 0/1 返回的下一页 token；page 2 返回 nil 结束
	idx := 0
	got, err := paginate(func(page *string) ([]int, *string, error) {
		if idx == 0 && page != nil {
			t.Errorf("first call page = %q, want nil", *page)
		}
		if idx > 0 {
			if page == nil || *page != tokens[idx-1] {
				t.Errorf("call %d page = %v, want %q", idx, page, tokens[idx-1])
			}
		}
		items := pages[idx]
		var next *string
		if idx < len(tokens) {
			n := tokens[idx]
			next = &n
		}
		idx++
		return items, next, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
	if idx != 3 {
		t.Errorf("page calls = %d, want 3", idx)
	}
}

// TestPaginateError 验证 B4：首页即出错时返回错误且不返回部分结果。
func TestPaginateError(t *testing.T) {
	sentinel := errors.New("boom")
	got, err := paginate(func(page *string) ([]int, *string, error) {
		return nil, nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want %v", err, sentinel)
	}
	if got != nil {
		t.Errorf("got = %v, want nil on error", got)
	}
}

// TestPaginateErrorMidStream 验证 B4：中途某页出错时立即返回错误。
func TestPaginateErrorMidStream(t *testing.T) {
	sentinel := errors.New("boom")
	calls := 0
	_, err := paginate(func(page *string) ([]int, *string, error) {
		calls++
		if calls == 2 {
			return nil, nil, sentinel
		}
		next := "next"
		return []int{calls}, &next, nil
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}
