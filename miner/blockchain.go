package miner

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
	"fmt"
	"github.com/oigele/bazo-miner/crypto"
	"github.com/oigele/bazo-miner/p2p"
	"github.com/oigele/bazo-miner/protocol"
	"github.com/oigele/bazo-miner/storage"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	logger                       *log.Logger
	blockValidation              = &sync.Mutex{}
	epochBlockValidation	     = &sync.Mutex{}
	stateTransitionValidation    = &sync.Mutex{}
	parameterSlice               []Parameters
	ActiveParameters 			*Parameters
	uptodate                     bool
	slashingDict                 = make(map[[32]byte]SlashingProof)
	validatorAccAddress          [64]byte
	hasher                       [32]byte
	multisigPubKey               *ecdsa.PublicKey
	commPrivKey, rootCommPrivKey *rsa.PrivateKey
	// This map keeps track of the validator assignment to the shards
	ValidatorShardMap *protocol.ValShardMapping
	NumberOfShards    int
	// This slice stores the hashes of the last blocks from the other shards, needed to create the next epoch block.
	LastShardHashes [][32]byte

	FirstStartCommittee  bool

	//Kursat Extras
	prevBlockIsEpochBlock bool
	FirstStartAfterEpoch  bool
	blockStartTime        int64
	syncStartTime         int64
	blockEndTime          int64
	totalSyncTime         int64
	NumberOfShardsDelayed int
)

//p2p First start entry point


func InitCommittee() {

	FirstStartAfterEpoch = true
	storage.IsCommittee = true

	//Set up logger.
	logger = storage.InitLogger()

	logger.Printf("\n\n\n" +
		"BBBBBBBBBBBBBBBBB               AAA               ZZZZZZZZZZZZZZZZZZZ     OOOOOOOOO\n" +
		"B::::::::::::::::B             A:::A              Z:::::::::::::::::Z   OO:::::::::OO\n" +
		"B::::::BBBBBB:::::B           A:::::A             Z:::::::::::::::::Z OO:::::::::::::OO\n" +
		"BB:::::B     B:::::B         A:::::::A            Z:::ZZZZZZZZ:::::Z O:::::::OOO:::::::O\n" +
		"  B::::B     B:::::B        A:::::::::A           ZZZZZ     Z:::::Z  O::::::O   O::::::O\n" +
		"  B::::B     B:::::B       A:::::A:::::A                  Z:::::Z    O:::::O     O:::::O\n" +
		"  B::::BBBBBB:::::B       A:::::A A:::::A                Z:::::Z     O:::::O     O:::::O\n" +
		"  B:::::::::::::BB       A:::::A   A:::::A              Z:::::Z      O:::::O     O:::::O\n" +
		"  B::::BBBBBB:::::B     A:::::A     A:::::A            Z:::::Z       O:::::O     O:::::O\n" +
		"  B::::B     B:::::B   A:::::AAAAAAAAA:::::A          Z:::::Z        O:::::O     O:::::O\n" +
		"  B::::B     B:::::B  A:::::::::::::::::::::A        Z:::::Z         O:::::O     O:::::O\n" +
		"  B::::B     B:::::B A:::::AAAAAAAAAAAAA:::::A    ZZZ:::::Z     ZZZZZO::::::O   O::::::O\n" +
		"BB:::::BBBBBB::::::BA:::::A             A:::::A   Z::::::ZZZZZZZZ:::ZO:::::::OOO:::::::O\n" +
		"B:::::::::::::::::BA:::::A               A:::::A  Z:::::::::::::::::Z OO:::::::::::::OO\n" +
		"B::::::::::::::::BA:::::A                 A:::::A Z:::::::::::::::::Z   OO:::::::::OO\n" +
		"BBBBBBBBBBBBBBBBBAAAAAAA                   AAAAAAAZZZZZZZZZZZZZZZZZZZ     OOOOOOOOO\n\n\n")

	logger.Printf("\n\n\n-------------------- START Committee Member ---------------------")
	logger.Printf("This Miners IP-Address: %v\n\n", p2p.Ipport)

	currentTargetTime = new(timerange)
	target = append(target, 13)

	parameterSlice = append(parameterSlice, NewDefaultParameters())
	ActiveParameters = &parameterSlice[0]
	storage.EpochLength = ActiveParameters.Epoch_length

	//Listen for incoming blocks from the network
	go incomingData()
	//Listen for incoming epoch blocks from the network
	go incomingEpochData()
	//Listen to state transitions for validation purposes
	go incomingStateData()

	//wait for the first epoch block
	for {
		time.Sleep(time.Second)
		if (lastEpochBlock != nil) {
			if (lastEpochBlock.Height >= 2) {
				logger.Printf("accepting the state of epoch block height: %d", lastEpochBlock.Height)
				storage.State = lastEpochBlock.State
				NumberOfShards = lastEpochBlock.NofShards
				break
			}
		}
	}
	FirstStartCommittee = true
	CommitteeMining(int(lastEpochBlock.Height))
}



func CommitteeMining(height int) {
	logger.Printf("---------------------------------------- Committee Mining for Epoch Height: %d ----------------------------------------", height)
	blockIDBoolMap := make(map[int]bool)
	for k, _ := range blockIDBoolMap {
		blockIDBoolMap[k] = false
	}

	//generate sequence of all shard IDs starting from 1
	shardIDs := makeRange(1,NumberOfShards)
	logger.Printf("Number of shards: %d\n",NumberOfShards)

	//generating the assignment data
	logger.Printf("before assigning transactions")
	for _, shardId := range shardIDs {
		var ta *protocol.TransactionAssignment
		var accTxs []*protocol.AccTx
		var stakeTxs []*protocol.StakeTx
		var fundsTxs []*protocol.FundsTx
		var dataTxs []*protocol.DataTx

		openTransactions := storage.ReadAllOpenTxs()

		//empty the assignment and all the slices
		ta = nil
		accTxs = nil
		stakeTxs = nil
		fundsTxs = nil
		dataTxs = nil

		//since shard number 1 writes the epoch block, it is required to process all acctx and stake tx
		//the other transactions are distributed to the shards based on the public address of the sender
		for _, openTransaction := range openTransactions {
			switch openTransaction.(type) {
			case *protocol.AccTx:
				if shardId == 1 {
					accTxs = append(accTxs, openTransaction.(*protocol.AccTx))
				}
			case *protocol.StakeTx:
				if shardId == 1 {
					stakeTxs = append(stakeTxs, openTransaction.(*protocol.StakeTx))
				}
			case *protocol.FundsTx:
				if shardId == assignTransactionToShard(openTransaction) {
				//if shardId == 1 {
					fundsTxs = append(fundsTxs, openTransaction.(*protocol.FundsTx))
				}
			case *protocol.DataTx:
				if shardId == assignTransactionToShard(openTransaction) {
					dataTxs = append(dataTxs, openTransaction.(*protocol.DataTx))
				}
			}
		}
		ta = protocol.NewTransactionAssignment(height, shardId, accTxs, stakeTxs, fundsTxs, dataTxs)

		logger.Printf("length of open transactions: %d", len(storage.ReadAllOpenTxs()))
		storage.AssignedTxMap[shardId] = ta
		logger.Printf("broadcasting assignment data for ShardId: %d", shardId)
		logger.Printf("Length of AccTx: %d, StakeTx: %d, FundsTx: %d, DataTx: %d", len(accTxs), len(stakeTxs), len(fundsTxs), len(dataTxs))
		broadcastAssignmentData(ta)
	}
	storage.AssignmentHeight = height
	logger.Printf("After assigning transactions")

	waitGroup := sync.WaitGroup{}

	//let the goroutine collect the state transitions in the background and contionue with the block collection
	waitGroup.Add(1)
	go fetchStateTransitionsForHeight(height+1, &waitGroup)


	//key: shard ID; value: Relative state of the corresponding shard
	relativeStatesToCheck := make(map[int]*protocol.RelativeState)

	//no block validation in the first round to make sure that the genesis block isn't checked
	if !FirstStartCommittee {
		logger.Printf("before block validation")
		for {
			//the committee member is now bootstrapped. In an infinite for-loop, perform its task
			blockStashForHeight := protocol.ReturnBlockStashForHeight(storage.ReceivedShardBlockStash, uint32(height+1))
			if len(blockStashForHeight) != 0 {
				logger.Printf("height being inspected: %d", height+1)
				logger.Printf("length of block stash for height: %d", len(blockStashForHeight))
				//Iterate through state transitions and apply them to local state, keep track of processed shards
				//Also perform some verification steps, i.e. proof of stake check
				for _, b := range blockStashForHeight {
					if blockIDBoolMap[b.ShardId] == false {

						blockIDBoolMap[b.ShardId] = true

						logger.Printf("Validation of block height: %d, ShardID: %d", b.Height, b.ShardId)

						//Check state contains beneficiary.
						acc, err := storage.GetAccount(b.Beneficiary)
						if err != nil {
							logger.Printf("Don't have the beneficiary")
							return
						}

						//Check if node is part of the validator set.
						if !acc.IsStaking {
							logger.Printf("Account isn't staking")
							return
						}

						//First, initialize an RSA Public Key instance with the modulus of the proposer of the block (acc)
						//Second, check if the commitment proof of the proposed block can be verified with the public key
						//Invalid if the commitment proof can not be verified with the public key of the proposer
						commitmentPubKey, err := crypto.CreateRSAPubKeyFromBytes(acc.CommitmentKey)
						if err != nil {
							logger.Printf("commitment key cannot be retrieved")
							return
						}

						err = crypto.VerifyMessageWithRSAKey(commitmentPubKey, fmt.Sprint(b.Height), b.CommitmentProof)
						logger.Printf("CommitmentPubKey: %x, --------------- Block Height: %d", commitmentPubKey, b.Height)
						if err != nil {
							logger.Printf("The submitted commitment proof can not be verified.")
							return
						}

						//Invalid if PoS calculation is not correct.
						prevProofs := GetLatestProofs(ActiveParameters.num_included_prev_proofs, b)
						if (validateProofOfStake(getDifficulty(), prevProofs, b.Height, acc.Balance, b.CommitmentProof, b.Timestamp)) {
							logger.Printf("proof of stake is valid")
						} else {
							logger.Printf("proof of stake is invalid")
						}


						accTxs, fundsTxs, _, stakeTxs, aggTxs, aggregatedFundsTxSlice, dataTxs, aggregatedDataTxSlice, aggDataTxs, err := preValidate(b, false)

						//append the aggTxs to the normal fundsTxs to delete
						fundsTxs = append(fundsTxs, aggregatedFundsTxSlice...)
						dataTxs = append(dataTxs, aggregatedDataTxSlice...)



						//here create the state copy and calculate the relative state
						//for this purpose, only the flow of funds has to be analyzed
						var StateCopy = CopyState(storage.State)
						var StateOld = CopyState(storage.State)


						//Shard 1 has more transactions to check
						//order matters
						//if b.ShardId == 1 {
						if true {
							StateCopy, _ = applyAccTxFeesAndCreateAccTx(StateCopy, b.Beneficiary, accTxs)
							StateCopy, _ = applyStakeTxFees(StateCopy, b.Beneficiary, stakeTxs)
							StateCopy, _ = applyFundsTxFeesFundsMovement(StateCopy, b.Beneficiary, fundsTxs)
						}

						//the fees are applied on the state copy
						StateCopy, _ = applyDataTxFees(StateCopy, b.Beneficiary, dataTxs)

						relativeStateProvisory := storage.GetRelativeStateForCommittee(StateOld, StateCopy)

						relativeState := protocol.NewRelativeState(relativeStateProvisory, b.ShardId)

						relativeStatesToCheck[b.ShardId] = relativeState


						//only iterate through data Txs once, so write summary AND check fee just once.
						if len(dataTxs) > 0 {
							err := storage.UpdateDataSummary(dataTxs); if err != nil {
								logger.Printf("Error when updating the data summary")
								return
							} else {
								logger.Printf("Data Summary Updated")
								newDataSummarySlice := storage.ReadAllDataSummary()
								if len(newDataSummarySlice) == 0 {
									logger.Printf("got a problem!!")
									return
								}
								//logger.Printf("Start Print data summary")
								//for _, dataSummary := range newDataSummarySlice {
									//logger.Printf(dataSummary.String())
								//}
							}
						}

						logger.Printf("In block from shardID: %d, height: %d, deleting accTxs: %d, stakeTxs: %d, fundsTxs: %d, aggTxs: %d, dataTxs: %d, aggDataTxs: %d", b.ShardId, b.Height, len(accTxs), len(stakeTxs), len(fundsTxs), len(aggTxs), len(dataTxs), len(aggDataTxs))

						storage.WriteAllClosedTx(accTxs, stakeTxs, fundsTxs, aggTxs, dataTxs, aggDataTxs)
						storage.DeleteAllOpenTx(accTxs, stakeTxs, fundsTxs, aggTxs, dataTxs, aggDataTxs)

						logger.Printf("Processed block of shard: %d\n", b.ShardId)

					}
				}
				//If all blocks have been received, stop synchronisation
				if len(blockStashForHeight) == NumberOfShards {
					logger.Printf("received all blocks for height. Break")
					break
				} else {
					logger.Printf("height: %d", height+1)
					logger.Printf("number of shards: %d", NumberOfShards)
				}
			}
			//for the blocks that haven't been processed yet, introduce request structure
			//can still accelerate this structure
			for _, shardIdReq := range shardIDs {
				if !blockIDBoolMap[shardIdReq] {
					var b *protocol.Block
					logger.Printf("Requesting Block for Height: %d and ShardID %d",int(height)+1, shardIdReq)
					p2p.ShardBlockReq(int(height)+1, shardIdReq)
					//blocking wait
					select {
					//received the response, perform the verification and write in map
					case encodedBlock := <-p2p.ShardBlockReqChan:
						b = b.Decode(encodedBlock)

						if b == nil {
							logger.Printf("block is nil")
						}

						if b.ShardId != shardIdReq {
							logger.Printf("Shard ID of received block %d vs shard ID of request %d. Continue", b.ShardId, shardIdReq)
							continue
						}
						blockIDBoolMap[shardIdReq] = true

						logger.Printf("Validation of block height: %d, ShardID: %d", b.Height, b.ShardId)

						//Check state contains beneficiary.
						acc, err := storage.GetAccount(b.Beneficiary)
						if err != nil {
							logger.Printf("Don't have the beneficiary")
							return
						}

						//Check if node is part of the validator set.
						if !acc.IsStaking {
							logger.Printf("Account isn't staking")
							return
						}

						//First, initialize an RSA Public Key instance with the modulus of the proposer of the block (acc)
						//Second, check if the commitment proof of the proposed block can be verified with the public key
						//Invalid if the commitment proof can not be verified with the public key of the proposer
						commitmentPubKey, err := crypto.CreateRSAPubKeyFromBytes(acc.CommitmentKey)
						if err != nil {
							logger.Printf("commitment key cannot be retrieved")
							return
						}

						err = crypto.VerifyMessageWithRSAKey(commitmentPubKey, fmt.Sprint(b.Height), b.CommitmentProof)
						logger.Printf("CommitmentPubKey: %x, --------------- Block Height: %d", commitmentPubKey, b.Height)
						if err != nil {
							logger.Printf("The submitted commitment proof can not be verified.")
							return
						}

						//Invalid if PoS calculation is not correct.
						prevProofs := GetLatestProofs(ActiveParameters.num_included_prev_proofs, b)
						if (validateProofOfStake(getDifficulty(), prevProofs, b.Height, acc.Balance, b.CommitmentProof, b.Timestamp)) {
							logger.Printf("proof of stake is valid")
						} else {
							logger.Printf("proof of stake is invalid")
						}


						accTxs, fundsTxs, _, stakeTxs, aggTxs, aggregatedFundsTxSlice, dataTxs, aggregatedDataTxSlice, aggDataTxs, err := preValidate(b, false)

						//append the aggTxs to the normal fundsTxs to delete
						fundsTxs = append(fundsTxs, aggregatedFundsTxSlice...)
						dataTxs = append(dataTxs, aggregatedDataTxSlice...)



						//here create the state copy and calculate the relative state
						//for this purpose, only the flow of funds has to be analyzed
						var StateCopy = CopyState(storage.State)
						var StateOld = CopyState(storage.State)

						//Shard 1 has more transactions to check
						//order matters
						//if b.ShardId == 1 {
						if true {
							StateCopy, _ = applyAccTxFeesAndCreateAccTx(StateCopy, b.Beneficiary, accTxs)
							StateCopy, _ = applyStakeTxFees(StateCopy, b.Beneficiary, stakeTxs)
							StateCopy, _ = applyFundsTxFeesFundsMovement(StateCopy, b.Beneficiary, fundsTxs)
						}


						//the fees are applied on the state copy
						StateCopy, _ = applyDataTxFees(StateCopy, b.Beneficiary, dataTxs)




						relativeStateProvisory := storage.GetRelativeStateForCommittee(StateOld, StateCopy)

						relativeState := protocol.NewRelativeState(relativeStateProvisory, b.ShardId)

						relativeStatesToCheck[b.ShardId] = relativeState

						//only iterate through data Txs once, so write summary AND check fee just once.
						if len(dataTxs) > 0 {
							err := storage.UpdateDataSummary(dataTxs); if err != nil {
								logger.Printf("Error when updating the data summary")
								return
							} else {
								logger.Printf("Data Summary Updated")
								newDataSummarySlice := storage.ReadAllDataSummary()
								if len(newDataSummarySlice) == 0 {
									logger.Printf("got a problem!!")
									return
								}
								//logger.Printf("Start Print data summary")
								//for _, dataSummary := range newDataSummarySlice {
									//logger.Printf(dataSummary.String())
								//}
							}
						}

						logger.Printf("In block from shardID: %d, height: %d, deleting accTxs: %d, stakeTxs: %d, fundsTxs: %d, aggTxs: %d, dataTxs: %d, aggDataTxs: %d", b.ShardId, b.Height, len(accTxs), len(stakeTxs), len(fundsTxs), len(aggTxs), len(dataTxs), len(aggDataTxs))

						storage.WriteAllClosedTx(accTxs, stakeTxs, fundsTxs, aggTxs, dataTxs, aggDataTxs)
						storage.DeleteAllOpenTx(accTxs, stakeTxs, fundsTxs, aggTxs, dataTxs, aggDataTxs)



						//store the block in the received block stash as well
						blockHash := b.HashBlock()
						if storage.ReceivedShardBlockStash.BlockIncluded(blockHash) == false {
							logger.Printf("Writing block to stash Shard ID: %v  - Height: %d - Hash: %x\n", b.ShardId, b.Height, blockHash[0:8])
							storage.ReceivedShardBlockStash.Set(blockHash, b)
						}

						logger.Printf("Processed block of shard: %d\n", b.ShardId)

					case <-time.After(2 * time.Second):
						logger.Printf("waited 2 seconds for lastblock height: %d, shardID: %d", int(height)+1, shardIdReq)
						logger.Printf("Broadcast Epoch Block to bootstrap new nodes")
						broadcastEpochBlock(lastEpochBlock)
					}
				}
			}
		}
		logger.Printf("end of block validation for height: %d", storage.AssignmentHeight)
	}

	//wait until the state transitions are all in storage.
	logger.Printf("Waiting for all state transitions to arrive")
	waitGroup.Wait()
	logger.Printf("All state transitions already received")

	//go through all state transitions and check them
	stateStashForHeight := protocol.ReturnStateTransitionForHeight(storage.ReceivedStateStash, uint32(height+1))
	for _,st := range stateStashForHeight {
		ownRelativeState := relativeStatesToCheck[st.ShardID]
		if !sameRelativeState(st.RelativeStateChange, ownRelativeState.RelativeState) {
			logger.Printf("FOUND A CHEATER: Shard %d", st.ShardID)
			//TODO include punishment
			return
		} else {
			logger.Printf("For Shard ID: %d the relative states match", st.ShardID)
		}
	}


	logger.Printf("Wait for next epoch block")
	//wait for next epoch block
	epochBlockReceived := false
	for !epochBlockReceived {
		logger.Printf("waiting for epoch block height %d", uint32(storage.AssignmentHeight)+1+EPOCH_LENGTH)
		newEpochBlock := <-p2p.EpochBlockReceivedChan
		logger.Printf("received the desired epoch block")
		if newEpochBlock.Height == uint32(storage.AssignmentHeight)+1+EPOCH_LENGTH {
			//broadcastEpochBlock(storage.ReadLastClosedEpochBlock())
			epochBlockReceived = true
			//think about when the new state should be updated. Important information: acc/staking
			//but for proof of stake, balance would also be important...


			//since it's safely not the first step of mining anymore, it's safe to perform proof of stake at this step
			err := validateEpochBlock(&newEpochBlock, relativeStatesToCheck)
			if err != nil {
				logger.Printf(err.Error())
				return
			} else {
				logger.Printf("The Epoch Block and its state are valid")
			}


			//before being able to validate the proof of stake, the state needs to updated
			storage.State = newEpochBlock.State
			ValidatorShardMap = newEpochBlock.ValMapping

			NumberOfShards = newEpochBlock.NofShards
		}
	}
	FirstStartCommittee = false
	logger.Printf("Received epoch block. Start next round")
	CommitteeMining(int(lastEpochBlock.Height))
}


func InitFirstStart(validatorWallet, multisigWallet, rootWallet *ecdsa.PublicKey, validatorCommitment, rootCommitment *rsa.PrivateKey) error {
	var err error
	if err != nil {
		return err
	}

	rootAddress := crypto.GetAddressFromPubKey(rootWallet)

	var rootCommitmentKey [crypto.COMM_KEY_LENGTH]byte
	copy(rootCommitmentKey[:], rootCommitment.N.Bytes())

	genesis := protocol.NewGenesis(rootAddress, rootCommitmentKey)
	storage.WriteGenesis(&genesis)

	/*Write First Epoch block chained to the genesis block*/
	initialEpochBlock := protocol.NewEpochBlock([][32]byte{genesis.Hash()}, 0)
	initialEpochBlock.Hash = initialEpochBlock.HashEpochBlock()
	FirstEpochBlock = initialEpochBlock
	initialEpochBlock.State = storage.State

	storage.WriteFirstEpochBlock(initialEpochBlock)

	storage.WriteClosedEpochBlock(initialEpochBlock)

	storage.DeleteAllLastClosedEpochBlock()
	storage.WriteLastClosedEpochBlock(initialEpochBlock)

	firstValMapping := protocol.NewMapping()
	initialEpochBlock.ValMapping = firstValMapping

	return Init(validatorWallet, multisigWallet, rootWallet, validatorCommitment, rootCommitment)
}


//Miner entry point
func Init(validatorWallet, multisigWallet, rootWallet *ecdsa.PublicKey, validatorCommitment, rootCommitment *rsa.PrivateKey) error {
	var err error

	validatorAccAddress = crypto.GetAddressFromPubKey(validatorWallet)
	multisigPubKey = multisigWallet
	commPrivKey = validatorCommitment
	rootCommPrivKey = rootCommitment
	storage.IsCommittee = false

	//Set up logger.
	logger = storage.InitLogger()
	hasher = protocol.SerializeHashContent(validatorAccAddress)
	logger.Printf("Acc hash is (%x)", hasher[0:8])
	logger.Printf("\n\n\n" +
		"BBBBBBBBBBBBBBBBB               AAA               ZZZZZZZZZZZZZZZZZZZ     OOOOOOOOO\n" +
		"B::::::::::::::::B             A:::A              Z:::::::::::::::::Z   OO:::::::::OO\n" +
		"B::::::BBBBBB:::::B           A:::::A             Z:::::::::::::::::Z OO:::::::::::::OO\n" +
		"BB:::::B     B:::::B         A:::::::A            Z:::ZZZZZZZZ:::::Z O:::::::OOO:::::::O\n" +
		"  B::::B     B:::::B        A:::::::::A           ZZZZZ     Z:::::Z  O::::::O   O::::::O\n" +
		"  B::::B     B:::::B       A:::::A:::::A                  Z:::::Z    O:::::O     O:::::O\n" +
		"  B::::BBBBBB:::::B       A:::::A A:::::A                Z:::::Z     O:::::O     O:::::O\n" +
		"  B:::::::::::::BB       A:::::A   A:::::A              Z:::::Z      O:::::O     O:::::O\n" +
		"  B::::BBBBBB:::::B     A:::::A     A:::::A            Z:::::Z       O:::::O     O:::::O\n" +
		"  B::::B     B:::::B   A:::::AAAAAAAAA:::::A          Z:::::Z        O:::::O     O:::::O\n" +
		"  B::::B     B:::::B  A:::::::::::::::::::::A        Z:::::Z         O:::::O     O:::::O\n" +
		"  B::::B     B:::::B A:::::AAAAAAAAAAAAA:::::A    ZZZ:::::Z     ZZZZZO::::::O   O::::::O\n" +
		"BB:::::BBBBBB::::::BA:::::A             A:::::A   Z::::::ZZZZZZZZ:::ZO:::::::OOO:::::::O\n" +
		"B:::::::::::::::::BA:::::A               A:::::A  Z:::::::::::::::::Z OO:::::::::::::OO\n" +
		"B::::::::::::::::BA:::::A                 A:::::A Z:::::::::::::::::Z   OO:::::::::OO\n" +
		"BBBBBBBBBBBBBBBBBAAAAAAA                   AAAAAAAZZZZZZZZZZZZZZZZZZZ     OOOOOOOOO\n\n\n")

	logger.Printf("\n\n\n-------------------- START MINER ---------------------")
	logger.Printf("This Miners IP-Address: %v\n\n", p2p.Ipport)
	time.Sleep(2 * time.Second)
	parameterSlice = append(parameterSlice, NewDefaultParameters())
	ActiveParameters = &parameterSlice[0]
	storage.EpochLength = ActiveParameters.Epoch_length

	//Initialize root key.
	initRootKey(rootWallet)
	if err != nil {
		logger.Printf("Could not create a root account.\n")
	}

	currentTargetTime = new(timerange)
	target = append(target, 13)

	logger.Printf("ActiveConfigParams: \n%v\n------------------------------------------------------------------------\n\nBAZO is Running\n\n", ActiveParameters)

	//this is used to generate the state with aggregated transactions.
	for _, tx := range storage.ReadAllBootstrapReceivedTransactions() {
		if tx != nil {
			storage.DeleteOpenTx(tx)
			storage.WriteClosedTx(tx)
		}
	}
	storage.DeleteBootstrapReceivedMempool()

	var initialBlock *protocol.Block


	//Listen for incoming epoch blocks from the network
	go incomingEpochData()
	//Listen for incoming assignments from the network
	go incomingTransactionAssignment()
	//Listen for incoming state transitions from the network
	go incomingStateData()


	//Since new validators only join after the currently running epoch ends, they do no need to download the whole shardchain history,
	//but can continue with their work after the next epoch block and directly set their state to the global state of the first received epoch block


	if (p2p.IsBootstrap()) {
		initialBlock, err = initState() //From here on, every validator should have the same state representation
		if err != nil {
			return err
		}
		lastBlock = initialBlock
	} else {
		for {
			//As the non-bootstrapping node, wait until I receive the last epoch block as well as the validator assignment
			// The global variables 'lastEpochBlock' and 'ValidatorShardMap' are being set when they are received by the network
			//seems the timeout is needed for nodes to be able to access
			time.Sleep(time.Second)
			if lastEpochBlock != nil {
				logger.Printf("First statement ok")
				if (lastEpochBlock.Height > 0) {
					storage.State = lastEpochBlock.State
					NumberOfShards = lastEpochBlock.NofShards
					ValidatorShardMap = lastEpochBlock.ValMapping
					storage.ThisShardID = ValidatorShardMap.ValMapping[validatorAccAddress] //Save my ShardID
					storage.ThisShardMap[int(lastEpochBlock.Height)] = storage.ThisShardID
					FirstStartAfterEpoch = true
					lastBlock = dummyLastBlock
					epochMining(lastEpochBlock.Hash, lastEpochBlock.Height) //start mining based on the received Epoch Block
					//set the ID to 0 such that there wont be any answers to requests that shouldnt be answered
					storage.ThisShardIDDelayed = 0
				}
			}
		}
	}

	logger.Printf("Active config params:%v\n", ActiveParameters)

	//Define number of shards based on the validators in the network
	NumberOfShards = DetNumberOfShards()
	logger.Printf("Number of Shards: %v", NumberOfShards)

	/*First validator assignment is done by the bootstrapping node, the others will be done based on PoS at the end of each epoch*/
	if (p2p.IsBootstrap()) {
		var validatorShardMapping = protocol.NewMapping()
		validatorShardMapping.ValMapping = AssignValidatorsToShards()
		validatorShardMapping.EpochHeight = int(lastEpochBlock.Height)
		ValidatorShardMap = validatorShardMapping
		logger.Printf("Validator Shard Mapping:\n")
		logger.Printf(validatorShardMapping.String())
	}

	storage.ThisShardID = ValidatorShardMap.ValMapping[validatorAccAddress]
	storage.ThisShardMap[int(lastEpochBlock.Height)] = storage.ThisShardID
	epochMining(lastBlock.Hash, lastBlock.Height)

	return nil
}

/**
Main function of Bazo which is running all the time with the goal of mining blocks.
*/
func epochMining(hashPrevBlock [32]byte, heightPrevBlock uint32) {

	var epochBlock *protocol.EpochBlock

	for {
		//Indicates that a validator newly joined Bazo after the current epoch, thus his 'lastBlock' variable is nil
		//and he continues directly with the mining of the first shard block
		if FirstStartAfterEpoch {
			logger.Printf("First start after Epoch. New miner successfully introduced to Bazo network")
			mining(hashPrevBlock, heightPrevBlock)
		}

		if (lastBlock.Height == uint32(lastEpochBlock.Height)+uint32(ActiveParameters.Epoch_length)) {
			if (storage.ThisShardID == 1) {

				shardIDs := makeRange(1,NumberOfShards)

				shardIDStateBoolMap := make(map[int]bool)
				for k, _ := range shardIDStateBoolMap {
					shardIDStateBoolMap[k] = false
				}

				for {
					//If there is only one shard, then skip synchronisation mechanism
					if (NumberOfShards == 1) {
						break
					}

					//Retrieve all state transitions from the local state with the height of my last block
					stateStashForHeight := protocol.ReturnStateTransitionForHeight(storage.ReceivedStateStash, lastBlock.Height)

					if (len(stateStashForHeight) != 0) {
						//Iterate through state transitions and apply them to local state, keep track of processed shards
						for _, st := range stateStashForHeight {
							if shardIDStateBoolMap[st.ShardID] == false && st.ShardID != storage.ThisShardID {

								//first check the commitment Proof. If it's invalid, continue the search
								err := validateStateTransition(st)
								if err != nil {
									logger.Printf(err.Error())
									continue
								}

								//Apply all relative account changes to my local state
								storage.State = storage.ApplyRelativeState(storage.State, st.RelativeStateChange)
								shardIDStateBoolMap[st.ShardID] = true
								logger.Printf("Processed state transition of shard: %d\n", st.ShardID)
							}
						}
						//If all state transitions have been received, stop synchronisation
						if (len(stateStashForHeight) == NumberOfShards-1) {
							logger.Printf("Received all transitions. Continue.")
							break
						} else {
							logger.Printf("Length of state stash: %d", len(stateStashForHeight))
						}
					}
					//Iterate over shard IDs to check which ones are still missing, and request them from the network
					for _,id := range shardIDs{
						if(id != storage.ThisShardID && shardIDStateBoolMap[id] == false){

							//Maybe the transition was received in the meantime. Then dont request it again.
							foundSt := searchStateTransition(id, int(lastBlock.Height))
							if foundSt != nil {
								logger.Printf("skip planned request for shardID %d", id)
								continue
							}

							var stateTransition *protocol.StateTransition

							logger.Printf("requesting state transition for lastblock height: %d\n",lastBlock.Height)

							p2p.StateTransitionReqShard(id,int(lastBlock.Height))
							//Blocking wait
							select {
							case encodedStateTransition := <-p2p.StateTransitionShardReqChan:
								stateTransition = stateTransition.DecodeTransition(encodedStateTransition)

								//first check the commitment Proof. If it's invalid, continue the search
								err := validateStateTransition(stateTransition)
								if err != nil {
									logger.Printf(err.Error())
									continue
								}

								//Apply state transition to my local state
								storage.State = storage.ApplyRelativeState(storage.State,stateTransition.RelativeStateChange)

								logger.Printf("Writing state back to stash Shard ID: %v  VS my shard ID: %v - Height: %d\n",stateTransition.ShardID,storage.ThisShardID,stateTransition.Height)
								storage.ReceivedStateStash.Set(stateTransition.HashTransition(),stateTransition)

								//Delete transactions from mempool, which were validated by the other shards

								shardIDStateBoolMap[stateTransition.ShardID] = true

								logger.Printf("Processed state transition of shard: %d\n",stateTransition.ShardID)

								//Limit waiting time to 5 seconds seconds before aborting.
							case <-time.After(2 * time.Second):
								logger.Printf("have been waiting for 5 seconds for lastblock height: %d\n",lastBlock.Height)
								//It the requested state transition has not been received, then continue with requesting the other missing ones
								continue
							}
						}
					}
				}

				//After the state transition mechanism is finished, perform the epoch block creation

				epochBlock = protocol.NewEpochBlock([][32]byte{lastBlock.Hash}, lastBlock.Height+1)
				logger.Printf("epochblock beingprocessed height: %d\n", epochBlock.Height)


				logger.Printf("Before finalizeEpochBlock() ---- Height: %d\n", epochBlock.Height)
				//Finalize creation of the epoch block. In case another epoch block was mined in the meantime, abort PoS here


				//add the beneficiary to the epoch block
				validatorAcc, err := storage.GetAccount(protocol.SerializeHashContent(validatorAccAddress))
				if err != nil {
					logger.Printf("problem with getting the validator acc")
				}

				validatorAccHash := validatorAcc.Hash()



				copy(epochBlock.Beneficiary[:], validatorAccHash[:])

				err = finalizeEpochBlock(epochBlock)

				logger.Printf("After finalizeEpochBlock() ---- Height: %d\n", epochBlock.Height)

				if err != nil {
					logger.Printf("%v\n", err)
				} else {
					logger.Printf("EPOCH BLOCK mined (%x)\n", epochBlock.Hash[0:8])
				}

				//Successfully mined epoch block
				if err == nil {
					logger.Printf("Broadcast epoch block (%x)\n", epochBlock.Hash[0:8])
					//Broadcast epoch block to other nodes such that they can update their validator-shard assignment
					broadcastEpochBlock(epochBlock)
					storage.WriteClosedEpochBlock(epochBlock)
					storage.DeleteAllLastClosedEpochBlock()
					storage.WriteLastClosedEpochBlock(epochBlock)
					lastEpochBlock = epochBlock

					logger.Printf("Created Validator Shard Mapping :\n")
					logger.Printf(ValidatorShardMap.String())
					logger.Printf("Inserting EPOCH BLOCK: %v\n", epochBlock.String())
					logger.Printf("Created Validator Shard Mapping :\n")
					logger.Printf(ValidatorShardMap.String() + "\n")


					for _, prevHash := range epochBlock.PrevShardHashes {
						//FileConnections.WriteString(fmt.Sprintf("'%x' -> 'EPOCH BLOCK: %x'\n", prevHash[0:15], epochBlock.Hash[0:15]))
						logger.Printf(`"Hash : %x \n Height : %d" -> "EPOCH BLOCK: \n Hash : %x \n Height : %d \nMPT : %x"`+"\n", prevHash[0:8], epochBlock.Height-1, epochBlock.Hash[0:8], epochBlock.Height, epochBlock.MerklePatriciaRoot[0:8])
						logger.Printf(`"EPOCH BLOCK: \n Hash : %x \n Height : %d \nMPT : %x"`+`[color = red, shape = box]`+"\n", epochBlock.Hash[0:8], epochBlock.Height, epochBlock.MerklePatriciaRoot[0:8])
					}
				}

				//Introduce some delay in case there was a fork of the epoch block.
				//Even though the states of both epoch blocks are the same, the validator-shard assignment is likely to be different
				//General rule: Accept the last received epoch block as the valid one.
				//Idea: We just accept the last received epoch block. There is no rollback for epoch blocks in place.
				//Kürsat hopes that the last received Epoch block will be the same for all blocks.
				//This pseudo sortition mechanism of waiting probably wont be needed anymore
				//time.Sleep(5 * time.Second)
			// I'm not shard number one so I just wait until I receive the next epoch block
			} else {
				//wait until epoch block is received
				epochBlockReceived := false
				for !epochBlockReceived {
					newEpochBlock := <- p2p.EpochBlockReceivedChan
					//the new epoch block from the channel is the epoch block that i need at the moment
					if newEpochBlock.Height == lastBlock.Height + 1 {
						epochBlockReceived = true
						// take over state
						storage.State = newEpochBlock.State
						ValidatorShardMap = newEpochBlock.ValMapping
						NumberOfShards = newEpochBlock.NofShards
						storage.ThisShardID = ValidatorShardMap.ValMapping[validatorAccAddress]
						storage.ThisShardMap[int(newEpochBlock.Height)] = storage.ThisShardID
						lastEpochBlock = &newEpochBlock
						logger.Printf("Received  last epoch block with height %d. Continue mining", lastEpochBlock.Height)
					}
				}
			}
			prevBlockIsEpochBlock = true
			firstEpochOver = true
			received := false
			//now delete old assignment and wait to receive the assignment from the committee
			storage.AssignedTxMempool = nil
			//Blocking wait
			logger.Printf("Wait for transaction assignment")
			for {
				select {
				case encodedTransactionAssignment := <-p2p.TransactionAssignmentReqChan:
					var transactionAssignment *protocol.TransactionAssignment
					transactionAssignment = transactionAssignment.DecodeTransactionAssignment(encodedTransactionAssignment)
					//got the transaction assignment for the wrong height. Request again.
					if transactionAssignment.Height != int(lastEpochBlock.Height) {
						time.Sleep(2 * time.Second)
						p2p.TransactionAssignmentReq(int(lastEpochBlock.Height), storage.ThisShardID)
						logger.Printf("Assignment height: %d vs epoch height %d", transactionAssignment.Height, epochBlock.Height)
						continue
					}
					//overwrite the previous mempool. Take the new transactions
					for _, transaction := range transactionAssignment.AccTxs {
						storage.AssignedTxMempool = append(storage.AssignedTxMempool, transaction)
					}
					for _, transaction := range transactionAssignment.StakeTxs {
						storage.AssignedTxMempool = append(storage.AssignedTxMempool, transaction)
					}
					for _, transaction := range transactionAssignment.FundsTxs {
						storage.AssignedTxMempool = append(storage.AssignedTxMempool, transaction)
					}
					for _, transaction := range transactionAssignment.DataTxs {
						storage.AssignedTxMempool = append(storage.AssignedTxMempool, transaction)
					}
					logger.Printf("Success. Received assignment for height: %d", transactionAssignment.Height)
					received = true
				case <-time.After(5 * time.Second):
					logger.Printf("Requesting transaction assignment for shard ID: %d with height: %d", storage.ThisShardID, lastEpochBlock.Height)
					p2p.TransactionAssignmentReq(int(lastEpochBlock.Height), storage.ThisShardID)
					//this is used to bootstrap the committee.
					broadcastEpochBlock(lastEpochBlock)
				}
				if received {
					break
				}
			}

			logger.Printf("received both my transaction assignment and the epoch block. can continue now")


			//Continue mining with the hash of the last epoch block
			mining(lastEpochBlock.Hash, lastEpochBlock.Height)
		} else if (lastEpochBlock.Height == lastBlock.Height+1) {
			prevBlockIsEpochBlock = true
			mining(lastEpochBlock.Hash, lastEpochBlock.Height) //lastblock was received before we started creation of next epoch block
		} else {
			mining(lastBlock.Hash, lastBlock.Height)
		}
	}
}

//Mining is a constant process, trying to come up with a successful PoW.
func mining(hashPrevBlock [32]byte, heightPrevBlock uint32) {

	logger.Printf("\n\n __________________________________________________ New Mining Round __________________________________________________")
	logger.Printf("Create Next Block")
	//This is the same mutex that is claimed at the beginning of a block validation. The reason we do this is
	//that before start mining a new block we empty the mempool which contains tx data that is likely to be
	//validated with block validation, so we wait in order to not work on tx data that is already validated
	//when we finish the block.
	blockValidation.Lock()
	currentBlock := newBlock(hashPrevBlock, [crypto.COMM_PROOF_LENGTH]byte{}, heightPrevBlock+1)

	//Set shard identifier in block (not necessary? It's already written inside the block)
	currentBlock.ShardId = storage.ThisShardID
	logger.Printf("This shard ID: %d", storage.ThisShardID)

	logger.Printf("Prepare Next Block")
	prepareBlock(currentBlock)
	blockValidation.Unlock()
	logger.Printf("Prepare Next Block --> Done")
	blockBeingProcessed = currentBlock
	logger.Printf("Finalize Next Block")
	err := finalizeBlock(currentBlock)

	logger.Printf("Finalize Next Block -> Done. Block height: %d", blockBeingProcessed.Height)
	if err != nil {
		logger.Printf("%v\n", err)
	} else {
		logger.Printf("Block mined (%x)\n", currentBlock.Hash[0:8])
	}

	if err == nil {
		err := validate(currentBlock, false)
		if err == nil {
			//only the shards which do not create the epoch block need to send out a state transition
			if storage.ThisShardID != 1 {
				commitmentProof, err := crypto.SignMessageWithRSAKey(commPrivKey, fmt.Sprint(currentBlock.Height))
				if err != nil {
					logger.Printf("Got a problem with creating the commimentProof.")
					return
				}
				stateTransition := protocol.NewStateTransition(storage.RelativeState,int(currentBlock.Height), storage.ThisShardID, commitmentProof)
				copy(stateTransition.CommitmentProof[0:crypto.COMM_PROOF_LENGTH], commitmentProof[:])
				storage.WriteToOwnStateTransitionkStash(stateTransition)
				broadcastStateTransition(stateTransition)
			}
			broadcastBlock(currentBlock)
			logger.Printf("Validated block (mined): %vState:\n%v", currentBlock, getState())
		} else {
			logger.Printf("Mined block (%x) could not be validated: %v\n", currentBlock.Hash[0:8], err)
		}
	}

	//Prints miner connections
	p2p.EmptyingiplistChan()
	p2p.PrintMinerConns()


	FirstStartAfterEpoch = false
	NumberOfShardsDelayed = NumberOfShards
	storage.ThisShardIDDelayed = storage.ThisShardID

}

//At least one root key needs to be set which is allowed to create new accounts.
func initRootKey(rootKey *ecdsa.PublicKey) error {
	address := crypto.GetAddressFromPubKey(rootKey)
	addressHash := protocol.SerializeHashContent(address)

	var commPubKey [crypto.COMM_KEY_LENGTH]byte
	copy(commPubKey[:], rootCommPrivKey.N.Bytes())

	rootAcc := protocol.NewAccount(address, [32]byte{}, ActiveParameters.Staking_minimum, true, commPubKey, nil, nil)
	storage.State[addressHash] = &rootAcc
	storage.RootKeys[addressHash] = &rootAcc

	return nil
}

/**
Number of Shards is determined based on the total number of validators in the network. Currently, the system supports only
one validator per shard, thus Number of Shards = Number of Validators.
*/
func DetNumberOfShards() (numberOfShards int) {
	return int(math.Ceil(float64(GetValidatorsCount()) / float64(ActiveParameters.validators_per_shard)))
}

/**
This function assigns the validators to the single shards in a random fashion. In case multiple validators per shard are supported,
they would be assigned to the shards uniformly.
*/
func AssignValidatorsToShards() map[[64]byte]int {

	logger.Printf("Assign validators to shards start")
	/*This map denotes which validator is assigned to which shard index*/
	validatorShardAssignment := make(map[[64]byte]int)

	/*Fill 'validatorAssignedMap' with the validators of the current state.
	The bool value indicates whether the validator has been assigned to a shard
	*/
	validatorSlices := make([][64]byte, 0)
	validatorAssignedMap := make(map[[64]byte]bool)
	for _, acc := range storage.State {
		if acc.IsStaking {
			validatorAssignedMap[acc.Address] = false
			validatorSlices = append(validatorSlices, acc.Address)
		}
	}

	/*Iterate over range of shards. At each index, select a random validators
	from the map above and set is bool 'assigned' to TRUE*/
	rand.Seed(time.Now().Unix())

	for j := 1; j <= int(ActiveParameters.validators_per_shard); j++ {
		for i := 1; i <= NumberOfShards; i++ {

			//finished the process of assigning the validators to shards
			if len(validatorSlices) == 0 {
				//The following code makes sure that the newly staking node gets to mine the next epoch block
				if newStakingNode != [64]byte{} {
					logger.Printf("There is a new staking node")
					shardID := validatorShardAssignment[newStakingNode]
					logger.Printf("Designated ShardID of the new staking node: %d", shardID)
					//ned to fix the shard assignment
					if shardID != 1 {
						for designatedValidator, _ := range validatorShardAssignment {
							if validatorShardAssignment[designatedValidator] == 1 {
								logger.Printf("Validator with the designated shard ID 1: %x", designatedValidator[0:8])
								validatorShardAssignment[designatedValidator] = shardID
								validatorShardAssignment[newStakingNode] = 1
								break
							}
						}
					}
				} else {
					logger.Printf("Content of new staking node: %x", newStakingNode)
				}
				return validatorShardAssignment
			}

			randomIndex := rand.Intn(len(validatorSlices))
			randomValidator := validatorSlices[randomIndex]

			//Assign validator to shard ID
			validatorShardAssignment[randomValidator] = i
			//Remove assigned validator from active list
			validatorSlices = removeValidator(validatorSlices, randomIndex)
		}
	}

	//The following code makes sure that the newly staking node gets to mine the next epoch block
	if newStakingNode != [64]byte{} {
		logger.Printf("There is a new staking node: %x", newStakingNode[0:8])
		shardID := validatorShardAssignment[newStakingNode]
		logger.Printf("Designated ShardID of the new staking node: %d", shardID)
		//ned to fix the shard assignment
		if shardID != 1 {
			for designatedValidator, _ := range validatorShardAssignment {
				if validatorShardAssignment[designatedValidator] == 1 {
					logger.Printf("Validator with the designated shard ID 1: %x", designatedValidator[0:8])
					validatorShardAssignment[designatedValidator] = shardID
					validatorShardAssignment[newStakingNode] = 1
					break
				}
			}
		} else {
			logger.Printf("Assignment should be correct without change")
		}
	}
	return validatorShardAssignment
}


func searchStateTransition(shardID int, height int) *protocol.StateTransition {
	stateStash := protocol.ReturnStateTransitionForHeight(storage.ReceivedStateStash, uint32(height))
	for _,st := range stateStash {
		if st.ShardID == shardID {
			return st
		}
	}
	return nil
}

func fetchStateTransitionsForHeight(height int, group *sync.WaitGroup) {
	// if there is only one, shard, then no state transitions will be in the system to be fetched
	logger.Printf("Start fetch staet transitions for height: %d. Number of Shards: %d", height, NumberOfShards)
	if NumberOfShards == 1 {
		group.Done()
		return
	} else {
		shardIDs := makeRange(1,NumberOfShards)
		shardIDStateBoolMap := make(map[int]bool)
		for k, _ := range shardIDStateBoolMap {
			shardIDStateBoolMap[k] = false
		}

		//start the mechanism
		for {
			//Retrieve all state transitions from the local state with the height of my last block
			stateStashForHeight := protocol.ReturnStateTransitionForHeight(storage.ReceivedStateStash, uint32(height))
			if (len(stateStashForHeight) != 0) {
				//Iterate through state transitions and apply them to local state, keep track of processed shards
				for _, st := range stateStashForHeight {
					if shardIDStateBoolMap[st.ShardID] == false  {
						//first check the commitment Proof. If it's invalid, continue the search
						err := validateStateTransition(st)
						if err != nil {
							logger.Printf("Cannot validate state transition")
							continue
						}
						//Apply all relative account changes to my local state
						shardIDStateBoolMap[st.ShardID] = true
					}
				}
				//If all state transitions have been received, stop synchronisation
				if (len(stateStashForHeight) == NumberOfShards-1) {
					logger.Printf("Received all transitions. Continue.")
					group.Done()
					return
				} else {
					logger.Printf("Length of state stash: %d", len(stateStashForHeight))
				}
				//Iterate over shard IDs to check which ones are still missing, and request them from the network
				for _,id := range shardIDs{
					if  shardIDStateBoolMap[id] == false{

						//Maybe the transition was received in the meantime. Then dont request it again.
						foundSt := searchStateTransition(id, height)
						if foundSt != nil {
							logger.Printf("skip planned request for shardID %d", id)
							continue
						}

						var stateTransition *protocol.StateTransition

						logger.Printf("requesting state transition for lastblock height: %d\n",height)

						p2p.StateTransitionReqShard(id,height)
						//Blocking wait
						select {
						case encodedStateTransition := <-p2p.StateTransitionShardReqChan:
							stateTransition = stateTransition.DecodeTransition(encodedStateTransition)

							//first check the commitment Proof. If it's invalid, continue the search
							err := validateStateTransition(stateTransition)
							if err != nil {
								logger.Printf(err.Error())
								continue
							}

							storage.ReceivedStateStash.Set(stateTransition.HashTransition(),stateTransition)


							shardIDStateBoolMap[stateTransition.ShardID] = true


							//Limit waiting time to 5 seconds seconds before aborting.
						case <-time.After(2 * time.Second):
							logger.Printf("have been waiting for 2 seconds for lastblock height: %d\n",height)
							//It the requested state transition has not been received, then continue with requesting the other missing ones
							continue
						}
					}
				}
			}
		}
	}
}


func applyDataTxFees(state map[[32]byte]protocol.Account, beneficiary [32]byte,  dataTxs []*protocol.DataTx) (map[[32]byte]protocol.Account,  error) {
	var err error
	//the beneficiary stays the same for one round
	minerAcc := state[beneficiary]
	for _, tx := range dataTxs {
		if minerAcc.Balance+tx.Fee > MAX_MONEY {
			err = errors.New("Fee amount would lead to balance overflow at the miner account.")
		}
		senderAcc := state[tx.From]

		senderAcc.Balance -= tx.Fee
		minerAcc.Balance += tx.Fee
		state[tx.From] = senderAcc
		state[beneficiary] = minerAcc
	}
	return state, err
}

func applyAccTxFeesAndCreateAccTx(state map[[32]byte]protocol.Account, beneficiary [32]byte,  accTxs []*protocol.AccTx) (map[[32]byte]protocol.Account,  error) {
	var err error
	//the beneficiary stays the same for one round
	minerAcc := state[beneficiary]
	for _, tx := range accTxs {
		if minerAcc.Balance+tx.Fee > MAX_MONEY {
			err = errors.New("Fee amount would lead to balance overflow at the miner account.")
		}
		//For accTxs, new funds have to be produced
		minerAcc.Balance += tx.Fee
		state[beneficiary] = minerAcc
		//create the account and add it to the account
		newAcc := protocol.NewAccount(tx.PubKey, tx.Issuer, 100000, false, [crypto.COMM_KEY_LENGTH]byte{}, tx.Contract, tx.ContractVariables)
		newAccHash := newAcc.Hash()
		state[newAccHash] = newAcc
	}
	return state, err
}

func applyStakeTxFees(state map[[32]byte]protocol.Account, beneficiary [32]byte,  stakeTxs []*protocol.StakeTx) (map[[32]byte]protocol.Account,  error) {
	var err error
	//the beneficiary stays the same for one round
	minerAcc := state[beneficiary]
	for _, tx := range stakeTxs {
		if minerAcc.Balance+tx.Fee > MAX_MONEY {
			err = errors.New("Fee amount would lead to balance overflow at the miner account.")
		}
		senderAcc := state[tx.Account]
		senderAcc.Balance -= tx.Fee
		minerAcc.Balance += tx.Fee
		state[tx.Account] = senderAcc
		state[beneficiary] = minerAcc
	}
	return state, err
}

func applyFundsTxFeesFundsMovement(state map[[32]byte]protocol.Account, beneficiary [32]byte,  fundsTxs []*protocol.FundsTx) (map[[32]byte]protocol.Account,  error) {
	var err error
	//the beneficiary stays the same for one round
	minerAcc := state[beneficiary]
	for _, tx := range fundsTxs {
		if minerAcc.Balance+tx.Fee > MAX_MONEY {
			err = errors.New("Fee amount would lead to balance overflow at the miner account.")
		}
		//Partition the process in case the sender/receiver are the same as the beneficiary
		//first handle amount
		senderAcc := state[tx.From]
		receiverAcc := state[tx.To]
		senderAcc.Balance -= tx.Amount
		receiverAcc.Balance += tx.Amount
		state[tx.To] = receiverAcc
		state[tx.From] = senderAcc
		//now handle the fee
		minerAcc := state[beneficiary]
		senderAcc = state[tx.From]
		senderAcc.Balance -= tx.Fee
		minerAcc.Balance += tx.Fee
		state[tx.From] = senderAcc
		state[beneficiary] = minerAcc
	}
	return state, err
}


func sameRelativeState(calculatedMap map[[32]byte]*protocol.RelativeAccount, receivedMap map[[32]byte]*protocol.RelativeAccount) bool {
	//at the moment, we only care about funds. This, however could be extended in the future
	for account, _ := range calculatedMap {
		if calculatedMap[account].Balance != receivedMap[account].Balance {
			logger.Printf("Calculated MAP: Account %x has balance %d ----- Received MAP has balance %d", account[0:8], calculatedMap[account].Balance, receivedMap[account].Balance)
			return false
		}
	}
	return true
}



//Helper functions

func removeValidator(inputSlice [][64]byte, index int) [][64]byte {
	inputSlice[index] = inputSlice[len(inputSlice)-1]
	inputSlice = inputSlice[:len(inputSlice)-1]
	return inputSlice
}

func makeRange(min, max int) []int {
	a := make([]int, max-min+1)
	for i := range a {
		a[i] = min + i
	}
	return a
}
