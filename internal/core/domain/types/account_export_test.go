package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportType_ToInt_ShouldSucceed_ForStream(t *testing.T) {
	// Given
	var unitUnderTest = Stream

	// When
	result, err := unitUnderTest.ToInt()

	// Then
	assert.Nil(t, err)
	assert.Equal(t, result, 1)
}

func TestExportType_ToInt_ShouldSucceed_ForService(t *testing.T) {
	// Given
	var unitUnderTest = Service

	// When
	result, err := unitUnderTest.ToInt()

	// Then
	assert.Nil(t, err)
	assert.Equal(t, result, 2)
}

func TestExportType_ToInt_ShouldReturnError_WhenUnknownType(t *testing.T) {
	// Given
	var unitUnderTest = ExportType("what")

	// When
	result, err := unitUnderTest.ToInt()

	// Then
	assert.ErrorContains(t, err, "unknown ExportType \"what\"")
	assert.Equal(t, result, 1)
}
