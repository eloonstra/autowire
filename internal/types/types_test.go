package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypeRef_Key(t *testing.T) {
	tests := []struct {
		name     string
		typeRef  TypeRef
		expected string
	}{
		{
			name:     "simple type",
			typeRef:  TypeRef{Name: "Foo"},
			expected: "Foo",
		},
		{
			name:     "pointer type",
			typeRef:  TypeRef{Name: "Foo", IsPointer: true},
			expected: "*Foo",
		},
		{
			name:     "imported type",
			typeRef:  TypeRef{Name: "Foo", ImportPath: "pkg/bar"},
			expected: "pkg/bar.Foo",
		},
		{
			name:     "imported pointer",
			typeRef:  TypeRef{Name: "Foo", ImportPath: "pkg/bar", IsPointer: true},
			expected: "*pkg/bar.Foo",
		},
		{
			name:     "builtin type",
			typeRef:  TypeRef{Name: "string"},
			expected: "string",
		},
		{
			name:     "nested import path",
			typeRef:  TypeRef{Name: "Config", ImportPath: "github.com/example/pkg/config"},
			expected: "github.com/example/pkg/config.Config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.typeRef.Key()
			assert.Equal(t, tt.expected, got)
		})
	}
}
