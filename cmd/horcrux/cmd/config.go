package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/strangelove-ventures/horcrux/signer"
	"gopkg.in/yaml.v2"
)

func init() {
	nodesCmd.AddCommand(addNodesCmd())
	nodesCmd.AddCommand(removeNodesCmd())
	configCmd.AddCommand(nodesCmd)

	peersCmd.AddCommand(addPeersCmd())
	peersCmd.AddCommand(removePeersCmd())
	configCmd.AddCommand(peersCmd)
	// TODO: config chain-id set
	configCmd.AddCommand(setChainIdCmd)

	configCmd.AddCommand(initCmd())
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Commands to configure the horcrux signer",
}

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init [chain-id] [chain-nodes]",
		Aliases: []string{"i"},
		Short:   "initalize configuration file and home directory if one doesn't already exist",
		Long: "initalize configuration file, use flags for cosigner configuration.\n\n" +
			"[chain-id] is the chain id of the chain to validate\n" +
			"[chain-nodes] is a comma seperated array of chain node addresses i.e.\n" +
			"tcp://chain-node-1:1234,tcp://chain-node-2:1234",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			cid := args[0]
			var cn []ChainNode
			if len(args) == 2 {
				cn, err = chainNodesFromArg(args[1])
				if err != nil {
					return err
				}
			}

			var home string // In root.go we end up with our
			if homeDir != "" {
				home = homeDir
			} else {
				home, _ = homedir.Dir()
				home = path.Join(home, ".horcrux")
			}

			if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
				return fmt.Errorf("%s is not empty, check for existing configuration and clear path before trying again", homeDir)
			}

			var cfg *Config
			cs, _ := cmd.Flags().GetBool("cosigner")
			if cs {
				p, _ := cmd.Flags().GetString("peers")
				threshold, _ := cmd.Flags().GetInt("threshold")
				peers, err := peersFromFlag(p)
				listen, _ := cmd.Flags().GetString("listen")
				timeout, _ := cmd.Flags().GetString("timeout")

				if err != nil {
					return err
				}
				cfg = &Config{
					HomeDir: home,
					ChainID: cid,
					CosignerConfig: &CosignerConfig{
						Threshold: threshold,
						P2PListen: listen,
						Peers:     peers,
						Timeout:   timeout,
					},
					ChainNodes: cn,
				}
				if err = validateCosignerConfig(cfg); err != nil {
					return err
				}
			} else {
				if len(cn) == 0 {
					return fmt.Errorf("must input at least one node")
				}
				cfg = &Config{
					HomeDir:    home,
					ChainID:    cid,
					ChainNodes: cn,
				}
				if err = validateSingleSignerConfig(cfg); err != nil {
					return err
				}
			}
			// create all directories up to the state directory
			if err = os.MkdirAll(path.Join(home, "state"), 0755); err != nil {
				return err
			}
			// create the config file
			if err = writeConfigFile(path.Join(home, "config.yaml"), cfg); err != nil {
				return err
			}

			// initialize state/{chainid}_priv_validator_state.json file
			if _, err = signer.LoadOrCreateSignState(path.Join(home, "state", fmt.Sprintf("%s_priv_validator_state.json", cid))); err != nil {
				return err
			}

			// if node is a cosigner initialize state/{chainid}_priv_validator_state.json file
			if cs {
				if _, err = signer.LoadOrCreateSignState(path.Join(home, "state", fmt.Sprintf("%s_share_sign_state.json", cid))); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolP("cosigner", "c", false, "set to initialize a cosigner node, requires --peers and --threshold")
	cmd.Flags().StringP("peers", "p", "", "cosigner peer addresses in format tcp://{addr}:{port}|{share-id} \n"+
		"(i.e. \"tcp://node-1:2222|2,tcp://node-2:2222|3\")")
	cmd.Flags().IntP("threshold", "t", 0, "indicate number of signatures required for threshold signature")
	cmd.Flags().StringP("listen", "l", "tcp://0.0.0.0:2222", "listen address of the signer")
	cmd.Flags().String("timeout", "1500ms", "configure cosigner rpc server timeout value, \n"+
		"accepts valid duration strings for Go's time.ParseDuration() e.g. 1s, 1000ms, 1.5m")
	return cmd
}

func writeConfigFile(path string, cfg *Config) error {
	return ioutil.WriteFile(path, cfg.MustMarshalYaml(), 0644)
}

func validateSingleSignerConfig(cfg *Config) error {
	if cfg.ChainID == "" {
		return fmt.Errorf("chain-id cannot be empty")
	}
	if len(cfg.ChainNodes) == 0 {
		return fmt.Errorf("need to have a node configured to sign for")
	}
	if err := validateChainNodes(cfg.ChainNodes); err != nil {
		return err
	}
	return nil
}

func validateCosignerConfig(cfg *Config) error {
	if cfg.ChainID == "" {
		return fmt.Errorf("chain-id cannot be empty")
	}
	if cfg.CosignerConfig == nil {
		return fmt.Errorf("cosigner config can't be empty")
	}
	if len(cfg.CosignerConfig.Peers)+1 < cfg.CosignerConfig.Threshold {
		return fmt.Errorf("number of peers + 1 (%d) must be greater than threshold (%d)", len(cfg.CosignerConfig.Peers)+1, cfg.CosignerConfig.Threshold)
	}

	_, err := time.ParseDuration(cfg.CosignerConfig.Timeout)
	if err != nil {
		return fmt.Errorf("%s is not a valid duration string for --timeout ", cfg.CosignerConfig.Timeout)
	}
	if _, err := url.Parse(cfg.CosignerConfig.P2PListen); err != nil {
		return fmt.Errorf("failed to parse p2p listen address")
	}
	if err := validateCosignerPeers(cfg.CosignerConfig.Peers); err != nil {
		return err
	}
	if err := validateChainNodes(cfg.ChainNodes); err != nil {
		return err
	}
	return nil
}

var nodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Commands to configure the chain nodes",
}

func addNodesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "add [chain-nodes]",
		Aliases: []string{"a"},
		Short:   "add chain node(s) to the cosigner's configuration",
		Long: "add chain node(s) to the cosigner's configuration.\n\n" +
			"[chain-nodes] is a comma seperated array of chain node addresses i.e.\n" +
			"tcp://chain-node-1:1234,tcp://chain-node-2:1234",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var home string // In root.go we end up with our
			if homeDir != "" {
				home = homeDir
			} else {
				home, _ = homedir.Dir()
				home = path.Join(home, ".horcrux")
			}
			if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
				return fmt.Errorf("%s is not empty, check for existing configuration and clear path before trying again", homeDir)
			}

			argNodes, err := chainNodesFromArg(args[0])
			if err != nil {
				return err
			}
			diffSet := diffSetChainNode(config.ChainNodes, argNodes)
			if len(diffSet) == 0 {
				return errors.New("no new chain nodes specified in args")
			}
			config.ChainNodes = append(config.ChainNodes, diffSet...)

			if err := writeConfigFile(path.Join(home, "config.yaml"), config); err != nil {
				return err
			}
			return nil
		},
	}
}

func removeNodesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove [chain-nodes]",
		Aliases: []string{"r"},
		Short:   "remove chain node(s) from the cosigner's configuration",
		Long: "remove chain node(s) from the cosigner's configuration.\n\n" +
			"[chain-nodes] is a comma seperated array of chain node addresses i.e.\n" +
			"tcp://chain-node-1:1234,tcp://chain-node-2:1234",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var home string // In root.go we end up with our
			if homeDir != "" {
				home = homeDir
			} else {
				home, _ = homedir.Dir()
				home = path.Join(home, ".horcrux")
			}
			if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
				return fmt.Errorf("%s is not empty, check for existing configuration and clear path before trying again", homeDir)
			}

			argNodes, err := chainNodesFromArg(args[0])
			if err != nil {
				return err
			}
			diffSet := diffSetChainNode(argNodes, config.ChainNodes)
			if len(diffSet) == 0 {
				return errors.New("cannot remove all chain nodes from config, please leave at least one")
			}
			config.ChainNodes = diffSet

			if err := writeConfigFile(path.Join(home, "config.yaml"), config); err != nil {
				return err
			}
			return nil
		},
	}
}

// diffSetChainNode decribes the difference set of setA-setB, which are all ChainNodes
// of setA that are also part of setB.
func diffSetChainNode(setA, setB []ChainNode) (diffSet []ChainNode) {
	for _, b := range setB {
		found := false
		for _, a := range setA {
			if b == a {
				found = true
			}
		}
		if !found {
			diffSet = append(diffSet, b)
		}
	}
	return
}

var peersCmd = &cobra.Command{
	Use:   "peers",
	Short: "Commands to configure the peer nodes",
}

func addPeersCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "add [peer-nodes]",
		Aliases: []string{"a"},
		Short:   "add peer node(s) to the cosigner's configuration",
		Long: "add peer node(s) to the cosigner's configuration.\n\n" +
			"[peer-nodes] is a comma seperated array of peer node addresses i.e.\n" +
			"tcp://peer-node-1:1234,tcp://peer-node-2:1234",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var home string // In root.go we end up with our
			if homeDir != "" {
				home = homeDir
			} else {
				home, _ = homedir.Dir()
				home = path.Join(home, ".horcrux")
			}
			if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
				return fmt.Errorf("%s is not empty, check for existing configuration and clear path before trying again", homeDir)
			}

			argPeers, err := peersFromFlag(args[0])
			if err != nil {
				return err
			}
			diffSet := diffSetCosignerPeer(config.CosignerConfig.Peers, argPeers)
			if len(diffSet) == 0 {
				return errors.New("no new peer nodes specified in args")
			}
			if err := validateCosignerPeers(diffSet); err != nil {
				fmt.Println(err)
			}
			config.CosignerConfig.Peers = append(config.CosignerConfig.Peers, diffSet...)

			if err := writeConfigFile(path.Join(home, "config.yaml"), config); err != nil {
				return err
			}
			return nil
		},
	}
}

func removePeersCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove [peer-nodes]",
		Aliases: []string{"r"},
		Short:   "remove peer node(s) from the cosigner's configuration",
		Long: "remove peer node(s) from the cosigner's configuration.\n\n" +
			"[peer-nodes] is a comma seperated array of peer node addresses i.e.\n" +
			"tcp://peer-node-1:1234,tcp://peer-node-2:1234",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var home string // In root.go we end up with our
			if homeDir != "" {
				home = homeDir
			} else {
				home, _ = homedir.Dir()
				home = path.Join(home, ".horcrux")
			}
			if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
				return fmt.Errorf("%s is not empty, check for existing configuration and clear path before trying again", homeDir)
			}

			argPeers, err := peersFromFlag(args[0])
			if err != nil {
				return err
			}
			diffSet := diffSetCosignerPeer(argPeers, config.CosignerConfig.Peers)
			if len(diffSet) == 0 {
				return errors.New("cannot remove all peer nodes from config, please leave at least one")
			}
			config.CosignerConfig.Peers = diffSet

			if err := writeConfigFile(path.Join(home, "config.yaml"), config); err != nil {
				return err
			}
			return nil
		},
	}
}

// diffSetCosignerPeer decribes the difference set of setA-setB, which are all
// CosignerPeers of setA that are also part of setB.
func diffSetCosignerPeer(setA, setB []CosignerPeer) (diffSet []CosignerPeer) {
	for _, b := range setB {
		found := false
		for _, a := range setA {
			if b == a {
				found = true
			}
		}
		if !found {
			diffSet = append(diffSet, b)
		}
	}
	return
}

var setChainIdCmd = &cobra.Command{
	Use:     "set-chain-id [chain-ID]",
	Aliases: []string{"id"},
	Short:   "set the chain ID",
	Long: "set the chain ID.\n\n" +
		"[chain-id] is a string i.e.\n" +
		"cosmoshub-4",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		var home string // In root.go we end up with our
		if homeDir != "" {
			home = homeDir
		} else {
			home, _ = homedir.Dir()
			home = path.Join(home, ".horcrux")
		}

		if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
			return fmt.Errorf("%s is not empty, check for existing configuration and clear path before trying again", homeDir)
		}

		config.ChainID = args[0]
		if err := writeConfigFile(path.Join(home, "config.yaml"), config); err != nil {
			return err
		}
		return nil
	},
}

type Config struct {
	HomeDir        string          `json:"home-dir" yaml:"home-dir"`
	ChainID        string          `json:"chain-id" yaml:"chain-id"`
	CosignerConfig *CosignerConfig `json:"cosigner,omitempty" yaml:"cosigner,omitempty"`
	ChainNodes     []ChainNode     `json:"chain-nodes,omitempty" yaml:"chain-nodes,omitempty"`
}

func (c *Config) Nodes() (out []signer.NodeConfig) {
	for _, n := range c.ChainNodes {
		out = append(out, signer.NodeConfig{Address: n.PrivValAddr})
	}
	return
}

func (c *Config) MustMarshalYaml() []byte {
	out, err := yaml.Marshal(c)
	if err != nil {
		panic(err)
	}
	return out
}

type CosignerConfig struct {
	Threshold int            `json:"threshold"   yaml:"threshold"`
	P2PListen string         `json:"p2p-listen"  yaml:"p2p-listen"`
	Peers     []CosignerPeer `json:"peers"       yaml:"peers"`
	Timeout   string         `json:"rpc-timeout" yaml:"rpc-timeout"`
}

func (c *Config) CosignerPeers() (out []signer.CosignerConfig) {
	for _, p := range c.CosignerConfig.Peers {
		out = append(out, signer.CosignerConfig{ID: p.ShareID, Address: p.P2PAddr})
	}
	return
}

type CosignerPeer struct {
	ShareID int    `json:"share-id" yaml:"share-id"`
	P2PAddr string `json:"p2p-addr" yaml:"p2p-addr"`
}

func readKeyShare() (*signer.CosignerKey, error) {
	// Read ID from local key share
	var home string // In root.go we end up with our
	if homeDir != "" {
		home = homeDir
	} else {
		home, _ = homedir.Dir()
		home = path.Join(home, ".horcrux")
	}

	if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s is not empty, check for existing configuration and clear path before trying again", homeDir)
	}

	file, err := os.Open(path.Join(home, "share.json"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bz, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	var key signer.CosignerKey
	if err := json.Unmarshal(bz, &key); err != nil {
		return nil, err
	}
	return &key, nil
}

func validateCosignerPeers(peers []CosignerPeer) error {
	// Check IDs to make sure none are duplicated
	encountered := make(map[int]string)
	duplicates := []int{}
	for _, peer := range peers {
		if _, found := encountered[peer.ShareID]; !found {
			encountered[peer.ShareID] = peer.P2PAddr
		} else {
			duplicates = append(duplicates, peer.ShareID)
		}
	}
	if len(duplicates) != 0 {
		return fmt.Errorf("found duplicates for peer IDs: %v", duplicates)
	}

	// TODO
	// key, err := readKeyShare()
	// if err != nil {
	// 	return err
	// }

	// // Check that the key share which is configured for the local node
	// // is the last remaining share
	// for _, peer := range config.CosignerPeers() {
	// 	if peer.ID == key.ID {
	// 		return fmt.Errorf("conflict: local node's key share ID %v matches with another peer in the config, please make sure you're using the correct share", key.ID)
	// 	}
	// }

	return nil
}

func peersFromFlag(peers string) (out []CosignerPeer, err error) {
	for _, p := range strings.Split(peers, ",") {
		ps := strings.Split(p, "|")
		if len(ps) != 2 {
			fmt.Println(ps)
			return nil, fmt.Errorf("invalid peer string %s", p)
		}
		shareid, err := strconv.ParseInt(ps[1], 10, 64)
		if err != nil {
			return nil, err
		}
		_, err = url.Parse(ps[0])
		if err != nil {
			return nil, err
		}
		out = append(out, CosignerPeer{ShareID: int(shareid), P2PAddr: ps[0]})
	}
	return
}

type ChainNode struct {
	PrivValAddr string `json:"priv-val-addr" yaml:"priv-val-addr"`
}

func chainNodesFromArg(arg string) ([]ChainNode, error) {
	cn := parseChainNodes(arg)
	return cn, validateChainNodes(cn)
}

func parseChainNodes(nodes string) (out []ChainNode) {
	for _, n := range strings.Split(nodes, ",") {
		out = append(out, ChainNode{PrivValAddr: n})
	}
	return
}

func validateChainNodes(nodes []ChainNode) (err error) {
	for _, n := range nodes {
		_, err = url.Parse(n.PrivValAddr)
	}
	return
}
