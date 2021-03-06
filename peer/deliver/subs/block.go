package subs

import (
	"context"
	"fmt"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/s7techlab/hlf-sdk-go/api"
	"github.com/s7techlab/hlf-sdk-go/util"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type blockSubscription struct {
	log         *zap.Logger
	channelName string
	identity    msp.SigningIdentity
	conn        *grpc.ClientConn
	client      peer.Deliver_DeliverClient
	blockChan   chan *common.Block
	errChan     chan error
	startPos    *orderer.SeekPosition
	stopPos     *orderer.SeekPosition
}

func (b *blockSubscription) handleSubscription() {

	log := b.log.Named(`handleSubscription`)

	log.Debug(`Starting subscription`)
	defer log.Debug(`Closing subscription`)

handleLoop:
	for {
		ev, err := b.client.Recv()
		log.Debug(`Got new DeliverResponse`)
		if err != nil {
			log.Fatal(err.Error())
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Canceled:
					log.Debug(`Got context.Canceled`)
					return
				default:
					log.Debug(`Got GRPC status code`, zap.Uint32(`grpc_code`, uint32(s.Code())), zap.String(`grpc_code_str`, s.Code().String()))
				}
			}

			log.Error(`Subscription error`, zap.Error(err))
			b.errChan <- &api.GRPCStreamError{Err: err}
			continue handleLoop
		}

		log.Debug(`Switch DeliverResponse Type`)
		switch event := ev.Type.(type) {
		case *peer.DeliverResponse_Block:
			log.Debug(`Got DeliverResponse_Block`,
				zap.Uint64(`number`, event.Block.Header.Number),
				zap.ByteString(`hash`, event.Block.Header.DataHash),
				zap.ByteString(`prevHash`, event.Block.Header.PreviousHash),
			)
			log.Debug(`Sending block to blockChan`)
			b.blockChan <- event.Block
			log.Debug(`Sent block to blockChan`)
		default:
			log.Debug(`Got DeliverResponse UnknownType`, zap.Reflect(`type`, ev.Type))
			b.errChan <- &api.UnknownEventTypeError{Type: fmt.Sprintf("%v", ev.Type)}
			log.Debug(`Sent err to errChan`)
		}
	}
}

func (b *blockSubscription) Blocks() chan *common.Block {

	return b.blockChan
}

func (b *blockSubscription) Errors() chan error {
	return b.errChan
}

func (b *blockSubscription) Close() error {

	log := b.log.Named(`Close`)

	log.Debug(`Closing errChan`)
	close(b.errChan)

	log.Debug(`Closing blockChan`)
	close(b.blockChan)

	log.Debug(`Trying to CloseSend of DeliverClient`)
	return b.client.CloseSend()
}

func NewBlockSubscription(ctx context.Context, channelName string, identity msp.SigningIdentity, conn *grpc.ClientConn, log *zap.Logger, seekOpt ...api.EventCCSeekOption) (api.BlockSubscription, error) {
	var startPos, stopPos *orderer.SeekPosition

	log = log.Named(`BlockSubscription`).
		With(zap.String(`channel`, channelName))

	if len(seekOpt) > 0 {
		startPos, stopPos = seekOpt[0]()
		log.Debug(`Using presented seekOpts`, zap.Reflect(`startPos`, startPos), zap.Reflect(`stopPos`, stopPos))
	} else {
		startPos, stopPos = api.SeekNewest()()
		log.Debug(`Using default seekOpts`, zap.Reflect(`startPos`, startPos), zap.Reflect(`stopPos`, stopPos))
	}

	log.Debug(`Initializing new DeliverClient`)

	cli, err := peer.NewDeliverClient(conn).Deliver(ctx)
	if err != nil {
		log.Error(`Initialization of DeliverClient failed`, zap.Error(err))
		return nil, errors.Wrap(err, `failed to create DeliverClient`)
	}

	log.Debug(`Getting seekEnvelope for DeliverClient`)
	if env, err := util.SeekEnvelope(channelName, startPos, stopPos, identity); err != nil {
		log.Error(`Getting seekEnvelope failed`, zap.Error(err))
		return nil, errors.Wrap(err, `failed to get seek envelope`)
	} else {
		log.Debug(`Got seekEnvelope`, zap.ByteString(`payload`, env.Payload), zap.ByteString(`signature`, env.Signature))
		log.Debug(`Sending seekEnvelope with DeliverClient`)
		if err = cli.Send(env); err != nil {
			log.Error(`Sending seekEnvelope failed`, zap.Error(err))
			return nil, errors.Wrap(err, `failed to send seek envelope`)
		}
	}

	sub := blockSubscription{
		log:         log,
		channelName: channelName,
		client:      cli,
		identity:    identity,
		conn:        conn,
		blockChan:   make(chan *common.Block),
		errChan:     make(chan error),
		startPos:    startPos,
		stopPos:     stopPos,
	}

	go sub.handleSubscription()

	return &sub, nil
}
