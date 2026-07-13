package vanilladata

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestExtract(t *testing.T) {
	data, ok := LoadDensityFunction("overworld/base_3d_noise")
	if !ok {
		t.Fatal("not found")
	}
	var pretty interface{}
	if err := json.Unmarshal(data, &pretty); err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(pretty, "", "  ")
	fmt.Println(string(b))
}
