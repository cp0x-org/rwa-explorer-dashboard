package compute

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// cmcInfoFixture decodes one token's info from the recorded /v2/cryptocurrency/info response in testdata/raw/cmc.
func cmcInfoFixture(t *testing.T, id string) model.CMCInfo {
	t.Helper()
	b, err := os.ReadFile("../../testdata/raw/cmc/crypto_info.json")
	require.NoError(t, err)
	var rec struct {
		Body struct {
			Data map[string]model.CMCInfo `json:"data"`
		} `json:"body"`
	}
	require.NoError(t, json.Unmarshal(b, &rec))
	info, ok := rec.Body.Data[id]
	require.True(t, ok, "id %s not in fixture", id)
	return info
}

// Moved from internal/source/cmc when ParseDeployments got its single home here: real CMC payloads.
func TestParseDeploymentsRecorded(t *testing.T) {
	settings, err := model.LoadSettings("../../config/settings.json") // the real chain_mapping (Arbitrum, Solana, ...)
	require.NoError(t, err)
	mapping := settings.ChainMapping

	arb := cmcInfoFixture(t, "28616")
	deps := ParseDeployments(&arb, mapping)
	require.Len(t, deps, 1) // token_address of the main platform duplicates contract_address[0]
	assert.Equal(t, "Arbitrum", deps[0].CMCNetwork)
	assert.Equal(t, "arbitrum", *deps[0].Network)
	assert.Equal(t, "0x4dafffddea93ddf1e0e7b61e844331455053ce5c", deps[0].Address)
	assert.Equal(t, "0x4DaFFfDDEa93DdF1e0e7B61E844331455053Ce5c", deps[0].AddressRaw)
	assert.Equal(t, "arbitrum:0x4dafffddea93ddf1e0e7b61e844331455053ce5c", *deps[0].Key)
	assert.Equal(t, "", deps[0].Status, "resolved later by the Portals lookup")

	nvdax := cmcInfoFixture(t, "36992")
	deps = ParseDeployments(&nvdax, mapping)
	require.NotEmpty(t, deps)
	var sol *Dep
	for i := range deps {
		if deps[i].CMCNetwork == "Solana" {
			sol = &deps[i]
		}
	}
	require.NotNil(t, sol)
	assert.Nil(t, sol.Key)
	assert.Equal(t, "unsupported_network", sol.Status)
	assert.Equal(t, sol.AddressRaw, sol.Address, "non-EVM addresses keep their case")
}

func TestParseDeploymentsSyntheticFromCMC(t *testing.T) {
	s := func(v string) *string { return &v }
	mapping := map[string]*string{"Ethereum": s("ethereum"), "Solana": nil}
	plat := func(name string) *struct {
		Name *string `json:"name"`
	} {
		return &struct {
			Name *string `json:"name"`
		}{Name: s(name)}
	}
	info := &model.CMCInfo{
		Platform: &model.CMCInfoPlatform{Name: s("Base"), TokenAddress: s("0xB20000000000000000000078EE7ce2fe4908108C")},
		ContractAddress: []model.CMCContractAddress{
			{ContractAddress: "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD", Platform: plat("Ethereum")},
			{ContractAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", Platform: plat("Ethereum")}, // dup
			{ContractAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", Platform: plat("Other")},    // other network
			{ContractAddress: "SoLAddr", Platform: plat("Solana")},
			{ContractAddress: "  ", Platform: plat("Ethereum")},
			{ContractAddress: "0x1", Platform: nil},
		},
	}
	deps := ParseDeployments(info, mapping)
	require.Len(t, deps, 4)
	assert.Equal(t, "ethereum:0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", *deps[0].Key)
	assert.Equal(t, "unmapped_network", deps[1].Status)
	assert.Nil(t, deps[1].Network)
	assert.Equal(t, "unsupported_network", deps[2].Status)
	assert.Equal(t, "SoLAddr", deps[2].Address)
	assert.Equal(t, "Base", deps[3].CMCNetwork) // platform fallback appended
	assert.Equal(t, "0xb20000000000000000000078ee7ce2fe4908108c", deps[3].Address)
	assert.Equal(t, "unmapped_network", deps[3].Status)
	assert.Nil(t, ParseDeployments(nil, mapping))
}
