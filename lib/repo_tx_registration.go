package lib

import (
	"context"

	"github.com/rs/zerolog/log"

	cosmossdk_io_math "cosmossdk.io/math"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
)

// True if the actor is ultimately, definitively registered for the specified topic, else False
// Idempotent in registration
func (node *NodeConfig) RegisterWorkerIdempotently(config WorkerConfig) bool {
	ctx := context.Background()

	isRegistered, err := node.IsWorkerRegistered(config.TopicId)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node is already registered for topic as worker, skipping")
		return false
	}
	if isRegistered {
		log.Info().Uint64("topicId", config.TopicId).Msg("Worker node already registered for topic")
		return true
	} else {
		log.Info().Uint64("topicId", config.TopicId).Msg("Worker node not yet registered for topic. Attempting registration...")
	}

	moduleParams, err := node.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
	if err != nil {
		log.Error().Err(err).Msg("Could not get chain params for worker ")
		return false
	}

	balance, err := node.GetBalance()
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the worker node has enough balance to register, skipping")
		return false
	}
	if !balance.GTE(moduleParams.Params.RegistrationFee) {
		log.Error().Str("balance", balance.String()).Msg("Worker node does not have enough balance to register, skipping.")
		return false
	}

	msg := &emissionstypes.RegisterRequest{
		Sender:    node.Chain.Address,
		TopicId:   config.TopicId,
		Owner:     node.Chain.Address,
		IsReputer: false,
	}
	res, err := node.SendDataWithRetry(ctx, msg, "Register worker node")
	if err != nil {
		txHash := ""
		if res != nil {
			txHash = res.TxHash
		}
		log.Error().Err(err).Uint64("topic", config.TopicId).Str("txHash", txHash).Msg("Could not register the worker node with the Allora blockchain")
		return false
	}

	isRegistered, err = node.IsWorkerRegistered(config.TopicId)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node is already registered for topic as worker, skipping")
		return false
	}

	return isRegistered
}

// True if the actor is ultimately, definitively registered for the specified topic with at least config.MinStake placed on topic, else False
// Actor may be either a worker or a reputer
// Idempotent in registration and stake addition
func (node *NodeConfig) RegisterAndStakeReputerIdempotently(config ReputerConfig) bool {
	ctx := context.Background()

	isRegistered, err := node.IsReputerRegistered(config.TopicId)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node is already registered for topic as reputer, skipping")
		return false
	}

	if isRegistered {
		log.Info().Uint64("topicId", config.TopicId).Msg("Reputer node already registered")
	} else {
		log.Info().Uint64("topicId", config.TopicId).Msg("Reputer node not yet registered. Attempting registration...")

		balance, err := node.GetBalance()
		if err != nil {
			log.Error().Err(err).Msg("Could not check if the Reputer node has enough balance to register, skipping")
			return false
		}
		moduleParams, err := node.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
		if err != nil {
			log.Error().Err(err).Msg("Could not get chain params for reputer")
			return false
		}
		if !balance.GTE(moduleParams.Params.RegistrationFee) {
			log.Error().Msg("Reputer node does not have enough balance to register, skipping.")
			return false
		}

		msgRegister := &emissionstypes.RegisterRequest{
			Sender:    node.Chain.Address,
			TopicId:   config.TopicId,
			Owner:     node.Chain.Address,
			IsReputer: true,
		}
		res, err := node.SendDataWithRetry(ctx, msgRegister, "Register reputer node")
		if err != nil {
			txHash := ""
			if res != nil {
				txHash = res.TxHash
			}
			log.Error().Err(err).Uint64("topic", config.TopicId).Str("txHash", txHash).Msg("Could not register the reputer node with the Allora blockchain")
			return false
		}

		isRegistered, err = node.IsReputerRegistered(config.TopicId)
		if err != nil {
			log.Error().Err(err).Msg("Could not check if the node is already registered for topic as reputer, skipping")
			return false
		}
		if !isRegistered {
			log.Error().Uint64("topicId", config.TopicId).Msg("Reputer node not registered after all retries")
			return false
		}
	}

	stake, err := node.GetReputerStakeInTopic(config.TopicId, node.Chain.Address)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the reputer node has enough balance to stake, skipping")
		return false
	}

	minStake := cosmossdk_io_math.NewInt(config.MinStake)
	if minStake.LTE(stake) {
		log.Info().Msg("Reputer stake above minimum requested stake, skipping adding stake.")
		return true
	} else {
		log.Info().Interface("stake", stake).Interface("minStake", minStake).Interface("stakeToAdd", minStake.Sub(stake)).Msg("Reputer stake below minimum requested stake, adding stake.")
	}

	msgAddStake := &emissionstypes.AddStakeRequest{
		Sender:  node.Wallet.Address,
		Amount:  minStake.Sub(stake),
		TopicId: config.TopicId,
	}
	res, err := node.SendDataWithRetry(ctx, msgAddStake, "Add reputer stake")
	if err != nil {
		txHash := ""
		if res != nil {
			txHash = res.TxHash
		}
		log.Error().Err(err).Uint64("topic", config.TopicId).Str("txHash", txHash).Msg("Could not stake the reputer node with the Allora blockchain in specified topic")
		return false
	}

	stake, err = node.GetReputerStakeInTopic(config.TopicId, node.Chain.Address)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the reputer node has enough balance to stake, skipping")
		return false
	}
	if stake.LT(minStake) {
		log.Error().Interface("stake", stake).Interface("minStake", minStake).Msg("Reputer stake below minimum requested stake, skipping.")
		return false
	}

	return true
}
