package chain

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// ERC20ABIJSON is the minimal ERC-20 ABI used for balance / metadata / approval.
const ERC20ABIJSON = `[
  {"name":"approve","type":"function","inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"type":"bool"}],"stateMutability":"nonpayable"},
  {"name":"allowance","type":"function","inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"outputs":[{"type":"uint256"}],"stateMutability":"view"},
  {"name":"balanceOf","type":"function","inputs":[{"name":"account","type":"address"}],"outputs":[{"type":"uint256"}],"stateMutability":"view"},
  {"name":"decimals","type":"function","inputs":[],"outputs":[{"type":"uint8"}],"stateMutability":"view"},
  {"name":"symbol","type":"function","inputs":[],"outputs":[{"type":"string"}],"stateMutability":"view"}
]`

// RouterABIJSON is the minimal Uniswap-V2 / PancakeSwap router ABI used by the
// swap and pricing paths.
const RouterABIJSON = `[
  {"name":"getAmountsOut","type":"function","inputs":[{"name":"amountIn","type":"uint256"},{"name":"path","type":"address[]"}],"outputs":[{"type":"uint256[]"}],"stateMutability":"view"},
  {"name":"WETH","type":"function","inputs":[],"outputs":[{"type":"address"}],"stateMutability":"pure"},
  {"name":"swapExactTokensForTokens","type":"function","inputs":[{"name":"amountIn","type":"uint256"},{"name":"amountOutMin","type":"uint256"},{"name":"path","type":"address[]"},{"name":"to","type":"address"},{"name":"deadline","type":"uint256"}],"outputs":[{"type":"uint256[]"}],"stateMutability":"nonpayable"},
  {"name":"swapExactETHForTokens","type":"function","inputs":[{"name":"amountOutMin","type":"uint256"},{"name":"path","type":"address[]"},{"name":"to","type":"address"},{"name":"deadline","type":"uint256"}],"outputs":[{"type":"uint256[]"}],"stateMutability":"payable"},
  {"name":"swapExactTokensForETH","type":"function","inputs":[{"name":"amountIn","type":"uint256"},{"name":"amountOutMin","type":"uint256"},{"name":"path","type":"address[]"},{"name":"to","type":"address"},{"name":"deadline","type":"uint256"}],"outputs":[{"type":"uint256[]"}],"stateMutability":"nonpayable"}
]`

// ERC20ABI is the parsed ERC-20 ABI, initialised at package load.
var ERC20ABI abi.ABI

// RouterABI is the parsed router ABI, initialised at package load.
var RouterABI abi.ABI

func init() {
	a, err := abi.JSON(strings.NewReader(ERC20ABIJSON))
	if err != nil {
		panic("parse ERC20 ABI: " + err.Error())
	}
	ERC20ABI = a

	a, err = abi.JSON(strings.NewReader(RouterABIJSON))
	if err != nil {
		panic("parse Router ABI: " + err.Error())
	}
	RouterABI = a
}
