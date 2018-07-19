package api

import (
	"fmt"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/peer"
	"google.golang.org/grpc"
)

// Peer is common interface for endorsing peer
type Peer interface {
	// Endorse sends proposal to endorsing peer and returns it's result
	Endorse(proposal *peer.SignedProposal) (*peer.ProposalResponse, error)
	// Uri returns url used for grpc connection
	Uri() string
	// Conn returns instance of grpc connection
	Conn() *grpc.ClientConn
	// Close terminates peer connection
	Close() error
}

// PeerProcessor is interface for processing transaction
type PeerProcessor interface {
	// CreateProposal creates signed proposal for presented cc, function and args using signing identity
	CreateProposal(cc *DiscoveryChaincode, identity msp.SigningIdentity, fn string, args [][]byte) (*peer.SignedProposal, ChaincodeTx, error)
	// Send sends signed proposal to endorsing peers and collects their responses
	Send(proposal *peer.SignedProposal, peers ...Peer) ([]*peer.ProposalResponse, error)
}

// PeerEndorseError describes peer endorse error
// TODO currently not working cause peer embeds error in string
type PeerEndorseError struct {
	Status  int32
	Message string
}

func (e PeerEndorseError) Error() string {
	return fmt.Sprintf("failed to endorse: %s (code: %d)", e.Message, e.Status)
}
