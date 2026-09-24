package validator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGherkinIncludes(t *testing.T) {
	contract, err := ParseGherkin("Feature: composed\nVersion: 3\nInclude: identity_required:2\n")
	require.NoError(t, err)
	require.Equal(t, []ContractInclude{{Name: "identity_required", Version: 2}}, contract.Includes)
	_, result := NewContractValidator().Validate("Feature: composed\nInclude: identity_required:2\n")
	require.False(t, result.IsValid, "unresolved imports must not validate as empty contracts")

	for _, source := range []string{
		"Feature: composed\nInclude: identity_required:0\n",
		"Feature: composed\nInclude: identity_required:abc\n",
		"Feature: composed\nInclude: identity_required:2\nInclude: identity_required:3\n",
		"Feature: composed\nRule: First\n  When step \"a\" is submitted\n  Then owner must be \"owner\"\nInclude: identity_required:2\n",
	} {
		_, err := ParseGherkin(source)
		require.Error(t, err, source)
	}
	_, err = ParseGherkin("Feature: composed\nInclude: identity_required:" + strings.Repeat("9", 100) + "\n")
	require.Error(t, err)
}
