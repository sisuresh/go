package effects

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestContractCreditedDestinationMuxedID guards the CAP-0084
// destination_muxed_id field on the contract_credited effect. The field is a
// decimal STRING (mirroring operations.AssetContractBalanceChange) precisely so
// that a mux id of "0" survives the JSON round-trip. A uint64 + ",string" +
// omitempty representation would unmarshal "0" to the zero value and then drop
// it on re-marshal, silently losing a valid id.
func TestContractCreditedDestinationMuxedID(t *testing.T) {
	for _, tc := range []struct {
		name        string
		muxedID     string
		wantPresent bool
	}{
		{name: "normal id", muxedID: "42", wantPresent: true},
		{name: "zero id round-trips", muxedID: "0", wantPresent: true},
		{name: "absent id is omitted", muxedID: "", wantPresent: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := ContractCredited{
				Contract:           "CB7KKMERLBFGQQILA4LM5VHDVL2RQVAQNLPMQHYBAM3J42UIIVZJSGEC",
				Amount:             "0.0012345",
				DestinationMuxedID: tc.muxedID,
			}

			data, err := json.Marshal(in)
			assert.NoError(t, err)

			// The JSON key is present iff the id is non-empty.
			var raw map[string]json.RawMessage
			assert.NoError(t, json.Unmarshal(data, &raw))
			_, present := raw["destination_muxed_id"]
			assert.Equal(t, tc.wantPresent, present)

			var out ContractCredited
			assert.NoError(t, json.Unmarshal(data, &out))
			assert.Equal(t, tc.muxedID, out.DestinationMuxedID)
		})
	}
}
