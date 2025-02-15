package corecontext

import (
	"fmt"
	"sort"
	"sync"

	lightclienttypes "github.com/zeta-chain/zetacore/x/lightclient/types"

	"github.com/rs/zerolog"
	"github.com/zeta-chain/zetacore/pkg/chains"
	observertypes "github.com/zeta-chain/zetacore/x/observer/types"
	"github.com/zeta-chain/zetacore/zetaclient/config"
)

// ZetaCoreContext contains core context params
// these are initialized and updated at runtime at every height
type ZetaCoreContext struct {
	coreContextLock    *sync.RWMutex
	keygen             observertypes.Keygen
	chainsEnabled      []chains.Chain
	evmChainParams     map[int64]*observertypes.ChainParams
	bitcoinChainParams *observertypes.ChainParams
	currentTssPubkey   string
	crossChainFlags    observertypes.CrosschainFlags

	// verificationFlags is used to store the verification flags for the lightclient module to enable header/proof verification
	verificationFlags lightclienttypes.VerificationFlags
}

// NewZetaCoreContext creates and returns new ZetaCoreContext
// it is initializing chain params from provided config
func NewZetaCoreContext(cfg config.Config) *ZetaCoreContext {
	evmChainParams := make(map[int64]*observertypes.ChainParams)
	for _, e := range cfg.EVMChainConfigs {
		evmChainParams[e.Chain.ChainId] = &observertypes.ChainParams{}
	}
	var bitcoinChainParams *observertypes.ChainParams
	_, found := cfg.GetBTCConfig()
	if found {
		bitcoinChainParams = &observertypes.ChainParams{}
	}
	return &ZetaCoreContext{
		coreContextLock:    new(sync.RWMutex),
		chainsEnabled:      []chains.Chain{},
		evmChainParams:     evmChainParams,
		bitcoinChainParams: bitcoinChainParams,
		crossChainFlags:    observertypes.CrosschainFlags{},
		verificationFlags:  lightclienttypes.VerificationFlags{},
	}
}

func (c *ZetaCoreContext) GetKeygen() observertypes.Keygen {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()

	var copiedPubkeys []string
	if c.keygen.GranteePubkeys != nil {
		copiedPubkeys = make([]string, len(c.keygen.GranteePubkeys))
		copy(copiedPubkeys, c.keygen.GranteePubkeys)
	}
	return observertypes.Keygen{
		Status:         c.keygen.Status,
		GranteePubkeys: copiedPubkeys,
		BlockNumber:    c.keygen.BlockNumber,
	}
}

func (c *ZetaCoreContext) GetCurrentTssPubkey() string {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()
	return c.currentTssPubkey
}

func (c *ZetaCoreContext) GetEnabledChains() []chains.Chain {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()
	copiedChains := make([]chains.Chain, len(c.chainsEnabled))
	copy(copiedChains, c.chainsEnabled)
	return copiedChains
}

func (c *ZetaCoreContext) GetEVMChainParams(chainID int64) (*observertypes.ChainParams, bool) {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()
	evmChainParams, found := c.evmChainParams[chainID]
	return evmChainParams, found
}

func (c *ZetaCoreContext) GetAllEVMChainParams() map[int64]*observertypes.ChainParams {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()

	// deep copy evm chain params
	copied := make(map[int64]*observertypes.ChainParams, len(c.evmChainParams))
	for chainID, evmConfig := range c.evmChainParams {
		copied[chainID] = &observertypes.ChainParams{}
		*copied[chainID] = *evmConfig
	}
	return copied
}

func (c *ZetaCoreContext) GetBTCChainParams() (chains.Chain, *observertypes.ChainParams, bool) {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()

	if c.bitcoinChainParams == nil { // bitcoin is not enabled
		return chains.Chain{}, &observertypes.ChainParams{}, false
	}
	chain := chains.GetChainFromChainID(c.bitcoinChainParams.ChainId)
	if chain == nil {
		panic(fmt.Sprintf("BTCChain is missing for chainID %d", c.bitcoinChainParams.ChainId))
	}
	return *chain, c.bitcoinChainParams, true
}

func (c *ZetaCoreContext) GetCrossChainFlags() observertypes.CrosschainFlags {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()
	return c.crossChainFlags
}

func (c *ZetaCoreContext) GetVerificationFlags() lightclienttypes.VerificationFlags {
	c.coreContextLock.RLock()
	defer c.coreContextLock.RUnlock()
	return c.verificationFlags
}

// Update updates core context and params for all chains
// this must be the ONLY function that writes to core context
func (c *ZetaCoreContext) Update(
	keygen *observertypes.Keygen,
	newChains []chains.Chain,
	evmChainParams map[int64]*observertypes.ChainParams,
	btcChainParams *observertypes.ChainParams,
	tssPubKey string,
	crosschainFlags observertypes.CrosschainFlags,
	verificationFlags lightclienttypes.VerificationFlags,
	init bool,
	logger zerolog.Logger,
) {
	c.coreContextLock.Lock()
	defer c.coreContextLock.Unlock()

	// Ignore whatever order zetabridge organizes chain list in state
	sort.SliceStable(newChains, func(i, j int) bool {
		return newChains[i].ChainId < newChains[j].ChainId
	})
	if len(newChains) == 0 {
		logger.Warn().Msg("UpdateChainParams: No chains enabled in ZeroCore")
	}

	// Add some warnings if chain list changes at runtime
	if !init {
		if len(c.chainsEnabled) != len(newChains) {
			logger.Warn().Msgf(
				"UpdateChainParams: ChainsEnabled changed at runtime!! current: %v, new: %v",
				c.chainsEnabled,
				newChains,
			)
		} else {
			for i, chain := range newChains {
				if chain != c.chainsEnabled[i] {
					logger.Warn().Msgf(
						"UpdateChainParams: ChainsEnabled changed at runtime!! current: %v, new: %v",
						c.chainsEnabled,
						newChains,
					)
				}
			}
		}
	}
	if keygen != nil {
		c.keygen = *keygen
	}
	c.chainsEnabled = newChains
	c.crossChainFlags = crosschainFlags
	c.verificationFlags = verificationFlags
	// update chain params for bitcoin if it has config in file
	if c.bitcoinChainParams != nil && btcChainParams != nil {
		c.bitcoinChainParams = btcChainParams
	}
	// update core params for evm chains we have configs in file
	for _, params := range evmChainParams {
		_, found := c.evmChainParams[params.ChainId]
		if !found {
			continue
		}
		c.evmChainParams[params.ChainId] = params
	}

	if tssPubKey != "" {
		c.currentTssPubkey = tssPubKey
	}
}

// IsOutboundObservationEnabled returns true if the chain is supported and outbound flag is enabled
func IsOutboundObservationEnabled(c *ZetaCoreContext, chainParams observertypes.ChainParams) bool {
	flags := c.GetCrossChainFlags()
	return chainParams.IsSupported && flags.IsOutboundEnabled
}

// IsInboundObservationEnabled returns true if the chain is supported and inbound flag is enabled
func IsInboundObservationEnabled(c *ZetaCoreContext, chainParams observertypes.ChainParams) bool {
	flags := c.GetCrossChainFlags()
	return chainParams.IsSupported && flags.IsInboundEnabled
}
