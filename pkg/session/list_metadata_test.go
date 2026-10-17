package session

import (
	"reflect"
	"strings"
	"testing"

	"excelsior/pkg/llm"
)

func TestListTitlesPreserveStoredRecords(t *testing.T) {
	disk := NewDirStore(t.TempDir())
	t.Cleanup(disk.Close)
	messages := []llm.Message{
		{Role: "assistant", Content: "Ignore this for the title"},
		{Role: "user", Content: "  "},
		{Role: "user", Content: "  " + strings.Repeat("界", 45) + "  "},
	}
	for name, store := range map[string]Store{"disk": disk, "memory": NewMemoryStore()} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				title, want string
				messages    []llm.Message
			}{
				{want: strings.Repeat("界", 40) + "…", messages: messages},
				{title: "  Custom title  ", want: "Custom title", messages: messages},
				{title: "  ", want: "New Chat"},
			} {
				record := Record{ID: "conversation", Title: tc.title, Messages: tc.messages}
				if err := store.Save(record); err != nil {
					t.Fatal(err)
				}
				before, err := store.Load(record.ID)
				if err != nil {
					t.Fatal(err)
				}
				metas, err := store.List()
				if err != nil || len(metas) != 1 {
					t.Fatalf("List = %v, %v", metas, err)
				}
				meta := metas[0]
				if meta.Title != tc.want || meta.MsgCount != len(tc.messages) || meta.UpdatedAt.IsZero() {
					t.Fatalf("unexpected metadata: %+v; want title %q", meta, tc.want)
				}
				after, err := store.Load(record.ID)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("List changed the stored record: %v", err)
				}
			}
		})
	}
}
