package cmd

import (
	"os"
	"path"
	"testing"

	"github.com/spf13/cobra"
	"github.com/strangelove-ventures/horcrux/signer"
	"github.com/stretchr/testify/require"
)

const (
	chainID = "horcrux-1"
)

func TestConfigInitCmd(t *testing.T) {
	tmpHome := "/tmp/TestConfigInitCmd"
	tmpConfig := path.Join(tmpHome, ".horcrux")

	err := os.Setenv("HOME", tmpHome)
	require.NoError(t, err)
	err = os.MkdirAll(tmpHome, 0777)
	require.NoError(t, err)

	tcs := []struct {
		name      string
		args      []string
		expectErr bool
	}{
		{
			name: "valid init",
			args: []string{
				chainID,
				"tcp://10.168.0.1:1234",
				"-c",
				"-p", "tcp://10.168.1.2:2222|2,tcp://10.168.1.3:2222|3",
				"--timeout", "1500ms",
			},
			expectErr: false,
		},
		{
			name: "invalid chain-nodes",
			args: []string{
				chainID,
				"://10.168.0.1:1234", // Missing/malformed protocol scheme
				"-c",
				"-p", "tcp://10.168.1.2:2222|2,tcp://10.168.1.3:2222|3",
				"--timeout", "1500ms",
			},
			expectErr: true,
		},
		{
			name: "invalid peer-nodes",
			args: []string{
				chainID,
				"tcp://10.168.0.1:1234",
				"-c",
				"-p", "tcp://10.168.1.2:2222,tcp://10.168.1.3:2222", // Missing share IDs
				"--timeout", "1500ms",
			},
			expectErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			cmd := initCmd()
			cmd.SetArgs(tc.args)
			err = cmd.Execute()
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				ss, err := signer.LoadSignState(path.Join(tmpConfig, "state", chainID+"_priv_validator_state.json"))
				require.NoError(t, err)
				require.Equal(t, int64(0), ss.Height)
				require.Equal(t, int64(0), ss.Round)
				require.Equal(t, int8(0), ss.Step)
				require.Nil(t, ss.EphemeralPublic)
				require.Nil(t, ss.Signature)
				require.Nil(t, ss.SignBytes)

				ss, err = signer.LoadSignState(path.Join(tmpConfig, "state", chainID+"_share_sign_state.json"))
				require.NoError(t, err)
				require.Equal(t, int64(0), ss.Height)
				require.Equal(t, int64(0), ss.Round)
				require.Equal(t, int8(0), ss.Step)
				require.Nil(t, ss.EphemeralPublic)
				require.Nil(t, ss.Signature)
				require.Nil(t, ss.SignBytes)
			}
		})
	}

	t.Cleanup(func() {
		err := os.RemoveAll(tmpHome)
		require.NoError(t, err)
	})
}

func TestConfigChainIDSetCmd(t *testing.T) {
	tmpHome := "/tmp/TestConfigChainIDSetCmd"

	err := os.Setenv("HOME", tmpHome)
	require.NoError(t, err)
	err = os.MkdirAll(tmpHome, 0777)
	require.NoError(t, err)

	cmd := initCmd()
	cmd.SetArgs([]string{
		chainID,
		"tcp://10.168.0.1:1234",
		"-c",
		"-p", "tcp://10.168.1.2:2222|2,tcp://10.168.1.3:2222|3",
		"--timeout", "1500ms",
	})
	err = cmd.Execute()
	require.NoError(t, err)

	tcs := []struct {
		name      string
		args      []string
		expectErr bool
	}{
		{
			name:      "happy path",
			args:      []string{"horcrux-2"},
			expectErr: false,
		},
		{
			name:      "missing chain-id",
			args:      []string{},
			expectErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			cmd := setChainIDCmd()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.args[0], config.ChainID)
			}
		})
	}

	t.Cleanup(func() {
		err := os.RemoveAll(tmpHome)
		require.NoError(t, err)
	})
}

func TestConfigNodesAddAndRemove(t *testing.T) {
	tmpHome := "/tmp/TestConfigNodesAddAndRemove"

	err := os.Setenv("HOME", tmpHome)
	require.NoError(t, err)
	err = os.MkdirAll(tmpHome, 0777)
	require.NoError(t, err)

	cmd := initCmd()
	cmd.SetArgs([]string{
		chainID,
		"tcp://10.168.0.1:1234",
		"-c",
		"-p", "tcp://10.168.1.1:2222|1,tcp://10.168.1.2:2222|2",
		"--timeout", "1500ms",
	})
	err = cmd.Execute()
	require.NoError(t, err)

	tcs := []struct {
		name        string
		cmd         *cobra.Command
		args        []string
		expectNodes []ChainNode
		expectErr   bool
	}{ // Do NOT change the order of the test cases!
		{
			name: "add single new node",
			cmd:  addNodesCmd(),
			args: []string{"tcp://10.168.0.2:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
				{PrivValAddr: "tcp://10.168.0.2:1234"},
			},
			expectErr: false,
		},
		{
			name: "remove single node",
			cmd:  removeNodesCmd(),
			args: []string{"tcp://10.168.0.2:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			expectErr: false,
		},
		{
			name: "add multiple new nodes",
			cmd:  addNodesCmd(),
			args: []string{"tcp://10.168.0.2:1234,tcp://10.168.0.3:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
				{PrivValAddr: "tcp://10.168.0.2:1234"},
				{PrivValAddr: "tcp://10.168.0.3:1234"},
			},
			expectErr: false,
		},
		{
			name: "remove multiple peers",
			cmd:  removeNodesCmd(),
			args: []string{"tcp://10.168.0.2:1234,tcp://10.168.0.3:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			expectErr: false,
		},
		{
			name: "add invalid node",
			cmd:  addNodesCmd(),
			args: []string{"://10.168.0.3:1234"}, // Missing/malformed protocol scheme
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			expectErr: true,
		},
		{
			name: "remove invalid node",
			cmd:  removeNodesCmd(),
			args: []string{"://10.168.0.3:1234"}, // Missing/malformed protocol scheme
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			expectErr: true,
		},
		{
			name: "add existing node",
			cmd:  addNodesCmd(),
			args: []string{"tcp://10.168.0.1:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			expectErr: true,
		},
		{
			name: "remove non-existent node",
			cmd:  removeNodesCmd(),
			args: []string{"tcp://10.168.0.99:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			expectErr: false,
		},
		{
			name: "add one new and one existing node",
			cmd:  addNodesCmd(),
			args: []string{"tcp://10.168.0.1:1234,tcp://10.168.0.2:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
				{PrivValAddr: "tcp://10.168.0.2:1234"},
			},
			expectErr: false,
		},
		{
			name: "remove one existing and one non-existent node",
			cmd:  removeNodesCmd(),
			args: []string{"tcp://10.168.0.2:1234,tcp://10.168.0.3:1234"},
			expectNodes: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			expectErr: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			tc.cmd.SetArgs(tc.args)
			err = tc.cmd.Execute()
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectNodes, config.ChainNodes)
		})
	}

	t.Cleanup(func() {
		err := os.RemoveAll(tmpHome)
		require.NoError(t, err)
	})
}

func TestConfigPeersAddAndRemove(t *testing.T) {
	tmpHome := "/tmp/TestConfigPeersAddAndRemove"

	err := os.Setenv("HOME", tmpHome)
	require.NoError(t, err)
	err = os.MkdirAll(tmpHome, 0777)
	require.NoError(t, err)

	cmd := initCmd()
	cmd.SetArgs([]string{
		chainID,
		"tcp://10.168.0.1:1234",
		"-c",
		"-p", "tcp://10.168.1.2:2222|2,tcp://10.168.1.3:2222|3,tcp://10.168.1.4:2222|4",
		"--timeout", "1500ms",
	})
	err = cmd.Execute()
	require.NoError(t, err)

	tcs := []struct {
		name        string
		cmd         *cobra.Command
		args        []string
		expectPeers []CosignerPeer
		expectErr   bool
	}{ // Do NOT change the order of the test cases!
		{
			name: "remove single peer",
			cmd:  removePeersCmd(),
			args: []string{"4"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
			},
			expectErr: false,
		},
		{
			name: "add single peer",
			cmd:  addPeersCmd(),
			args: []string{"tcp://10.168.1.4:2222|4"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
				{ShareID: 4, P2PAddr: "tcp://10.168.1.4:2222"},
			},
			expectErr: false,
		},
		{
			name: "remove multiple peers",
			cmd:  removePeersCmd(),
			args: []string{"3,4"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
			},
			expectErr: false,
		},
		{
			name: "add multiple peers",
			cmd:  addPeersCmd(),
			args: []string{"tcp://10.168.1.3:2222|3,tcp://10.168.1.4:2222|4"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
				{ShareID: 4, P2PAddr: "tcp://10.168.1.4:2222"},
			},
			expectErr: false,
		},
		{
			name: "remove non-existent peer",
			cmd:  removePeersCmd(),
			args: []string{"1"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
				{ShareID: 4, P2PAddr: "tcp://10.168.1.4:2222"},
			},
			expectErr: false,
		},
		{
			name: "add existing peer",
			cmd:  addPeersCmd(),
			args: []string{"tcp://10.168.1.3:2222|3"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
				{ShareID: 4, P2PAddr: "tcp://10.168.1.4:2222"},
			},
			expectErr: true,
		},
		{
			name: "remove one existing and one non-existent peer",
			cmd:  removePeersCmd(),
			args: []string{"1,4"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
			},
			expectErr: false,
		},
		{
			name: "add one non-existent and one existing peer",
			cmd:  addPeersCmd(),
			args: []string{"tcp://10.168.1.3:2222|3,tcp://10.168.1.4:2222|4"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
				{ShareID: 4, P2PAddr: "tcp://10.168.1.4:2222"},
			},
			expectErr: false,
		},
		{
			name: "add peer with ID out of range",
			cmd:  addPeersCmd(),
			args: []string{"tcp://10.168.1.5:2222|5"},
			expectPeers: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
				{ShareID: 4, P2PAddr: "tcp://10.168.1.4:2222"},
			},
			expectErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			tc.cmd.SetArgs(tc.args)
			err = tc.cmd.Execute()
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectPeers, config.CosignerConfig.Peers)
		})
	}

	t.Cleanup(func() {
		err := os.RemoveAll(tmpHome)
		require.NoError(t, err)
	})
}

func TestDiffSetChainNode(t *testing.T) {
	tcs := []struct {
		name       string
		setA       []ChainNode
		setB       []ChainNode
		expectDiff []ChainNode
	}{
		{
			name: "1 new, no overlap",
			setA: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			setB: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.2:1234"},
			},
			expectDiff: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
		},
		{
			name: "1 new, 1 overlap chain node",
			setA: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
				{PrivValAddr: "tcp://10.168.0.2:1234"},
			},
			setB: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.2:1234"},
				{PrivValAddr: "tcp://10.168.0.3:1234"},
			},
			expectDiff: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
		},
		{
			name: "0 new, partial overlap",
			setA: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
			},
			setB: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
				{PrivValAddr: "tcp://10.168.0.2:1234"},
			},
			expectDiff: nil,
		},
		{
			name: "0 new, all overlap",
			setA: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
				{PrivValAddr: "tcp://10.168.0.2:1234"},
			},
			setB: []ChainNode{
				{PrivValAddr: "tcp://10.168.0.1:1234"},
				{PrivValAddr: "tcp://10.168.0.2:1234"},
			},
			expectDiff: nil,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			diff := diffSetChainNode(tc.setA, tc.setB)
			require.Equal(t, diff, tc.expectDiff)
		})
	}
}

func TestDiffSetCosignerPeer(t *testing.T) {
	tcs := []struct {
		name       string
		setA       []CosignerPeer
		setB       []CosignerPeer
		expectDiff []CosignerPeer
	}{
		{
			name: "1 new, no overlap",
			setA: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
			},
			setB: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
			},
			expectDiff: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
			},
		},
		{
			name: "1 new, 1 overlap peer node",
			setA: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
			},
			setB: []CosignerPeer{
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
				{ShareID: 3, P2PAddr: "tcp://10.168.1.3:2222"},
			},
			expectDiff: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
			},
		},
		{
			name: "0 new, partial overlap",
			setA: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
			},
			setB: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
			},
			expectDiff: nil,
		},
		{
			name: "0 new, all overlap",
			setA: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
			},
			setB: []CosignerPeer{
				{ShareID: 1, P2PAddr: "tcp://10.168.1.1:2222"},
				{ShareID: 2, P2PAddr: "tcp://10.168.1.2:2222"},
			},
			expectDiff: nil,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			diff := diffSetCosignerPeer(tc.setA, tc.setB)
			require.Equal(t, diff, tc.expectDiff)
		})
	}
}
