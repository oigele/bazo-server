package miner

import (
	"errors"
	"fmt"
	"github.com/oigele/bazo-miner/crypto"
	"github.com/oigele/bazo-miner/p2p"
	"github.com/oigele/bazo-miner/protocol"
	"github.com/oigele/bazo-miner/storage"
	"os"
	"sync"
	"testing"
	"time"
)

const(
	NodesDirectory 		= "nodes/"
)

var (
	NodeNames			[]string
	TotalNodes			int
)

func TestGenerateNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestGenerateNodes...")
	} else {
		fmt.Printf("Generating nodes for testing \n")
	}
	TotalNodes = 10

	//Generate wallet directories for all nodes, i.e., validators and non-validators
	for i := 1; i <= TotalNodes; i++ {
		strNode := fmt.Sprintf("Node_%d",i)
		fmt.Printf("Generating node %d \n", i)
		if(!stringAlreadyInSlice(NodeNames,strNode)){
			NodeNames = append(NodeNames,strNode)
		}
		if _, err := os.Stat(NodesDirectory+strNode); os.IsNotExist(err) {
			err = os.MkdirAll(NodesDirectory+strNode, 0755)
			if err != nil {
				t.Errorf("Error while creating node directory %v\n",err)
			}
		}
		storage.Init(NodesDirectory+strNode+"/storage.db", TestIpPort)
		_, err := crypto.ExtractECDSAPublicKeyFromFile(NodesDirectory+strNode+"/wallet.key")
		if err != nil {
			return
		}
		_, err = crypto.ExtractRSAKeyFromFile(NodesDirectory+strNode+"/commitment.key")
		if err != nil {
			return
		}
	}
}

func TestShardIterative(t *testing.T) {
	rootNode := fmt.Sprintf("WalletA.txt")
	rootNodePubKey, _ := crypto.ExtractECDSAPublicKeyFromFile(rootNode)
	rootNodePrivKey, _ := crypto.ExtractECDSAKeyFromFile(rootNode)
	rootNodeAddress := crypto.GetAddressFromPubKey(rootNodePubKey)
	hasherRootNode := protocol.SerializeHashContent(rootNodeAddress)

	fromPrivKey, _ := crypto.ExtractECDSAKeyFromFile(rootNode)
	var nodeList [][32]byte

	//create 10 new accounts
	for i := 1; i <= 10; i++ {

		accTx, newAccAddress, err := protocol.ConstrAccTx(
			byte(0),
			uint64(1),
			[64]byte{},
			rootNodePrivKey,
			nil,
			nil)

		if err != nil {
			t.Log("got an issue")
		}

		if err := SendTx("127.0.0.1:8000", accTx, p2p.ACCTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}
		if err := SendTx("127.0.0.1:8001", accTx, p2p.ACCTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}

		newNodeAddress := crypto.GetAddressFromPubKey(&newAccAddress.PublicKey)
		hasherNewNode := protocol.SerializeHashContent(newNodeAddress)

		//append all 20 hashers to a new list
		nodeList = append(nodeList, hasherNewNode)

		t.Log(hasherNewNode)
		time.Sleep(10 * time.Second)

	}

	time.Sleep(200 * time.Second)

	i := 1

	for _, hasherNewNode := range nodeList {


		//send funds to new account
		tx, _ := protocol.ConstrFundsTx(
			byte(0),
			uint64(1000000),
			uint64(1),
			uint32(i),
			hasherRootNode,
			hasherNewNode,
			fromPrivKey,
			fromPrivKey,
			nil)

		if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
			t.Log(fmt.Sprintf("Error"))
		}
		if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
			t.Log(fmt.Sprintf("Error"))
		}

		i += 1

		time.Sleep(2 * time.Second)

	}

	time.Sleep(200 * time.Second)


	start := time.Now()

	//Sending all TX sequentially
	for _, hasher := range nodeList {
		for txCount := 0; txCount < 10000; txCount++ {
			tx, _ := protocol.ConstrFundsTx(
				byte(0),
				uint64(10),
				uint64(1),
				uint32(txCount),
				hasher,
				hasherRootNode,
				fromPrivKey,
				fromPrivKey,
				nil)

			if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
				t.Log(fmt.Sprintf("Error"))
			}
			if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
				t.Log(fmt.Sprintf("Error"))
			}
		}
	}


	t.Log("Waiting for goroutines to finish")

	elapsed := time.Now().Sub(start)

	t.Log(elapsed.Seconds())
	t.Log(elapsed.Nanoseconds())


}


func TestShard(t *testing.T) {

	rootNode := fmt.Sprintf("WalletA.txt")
	rootNodePubKey, _ := crypto.ExtractECDSAPublicKeyFromFile(rootNode)
	rootNodePrivKey, _ := crypto.ExtractECDSAKeyFromFile(rootNode)
	rootNodeAddress := crypto.GetAddressFromPubKey(rootNodePubKey)
	hasherRootNode := protocol.SerializeHashContent(rootNodeAddress)

	fromPrivKey, _ := crypto.ExtractECDSAKeyFromFile(rootNode)
	var nodeList [][32]byte

	//create 10 new accounts
	for i := 1; i <= 10; i++ {

		accTx, newAccAddress, err := protocol.ConstrAccTx(
			byte(0),
			uint64(1),
			[64]byte{},
			rootNodePrivKey,
			nil,
			nil)

		if err != nil {
			t.Log("got an issue")
		}

		if err := SendTx("127.0.0.1:8000", accTx, p2p.ACCTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}
		if err := SendTx("127.0.0.1:8001", accTx, p2p.ACCTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}

		newNodeAddress := crypto.GetAddressFromPubKey(&newAccAddress.PublicKey)
		hasherNewNode := protocol.SerializeHashContent(newNodeAddress)

		//append all 20 hashers to a new list
		nodeList = append(nodeList, hasherNewNode)

		t.Log(hasherNewNode)
		time.Sleep(10 * time.Second)

	}

	//time.Sleep(200 * time.Second)

	i := 1

	for _, hasherNewNode := range nodeList {


		//send funds to new account
		tx, _ := protocol.ConstrFundsTx(
			byte(0),
			uint64(1000000),
			uint64(1),
			uint32(i),
			hasherRootNode,
			hasherNewNode,
			fromPrivKey,
			fromPrivKey,
			nil)

		if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
			t.Log(fmt.Sprintf("Error"))
		}
		if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
			t.Log(fmt.Sprintf("Error"))
		}

		i += 1

		time.Sleep(2 * time.Second)

	}


	var wg sync.WaitGroup



	wg.Add(len(nodeList))

	start := time.Now()

	for _, hasher := range nodeList {
		go func([32]byte, *sync.WaitGroup) {
			defer wg.Done()
			t.Log(hasher[0:8])
			//now send tx back to the root account. Note that I never did anything manually with the new account
			for txCount := 0; txCount < 10000; txCount++ {
				tx, _ := protocol.ConstrFundsTx(
					byte(0),
					uint64(10),
					uint64(1),
					uint32(txCount),
					hasher,
					hasherRootNode,
					fromPrivKey,
					fromPrivKey,
					nil)

				if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
					t.Log(fmt.Sprintf("Error"))
				}
				if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
					t.Log(fmt.Sprintf("Error"))
				}
				if txCount == 9999 {
					t.Log(hasher[0:8])
				}
			}
			t.Log("One is finished")
		}(hasher, &wg)
		time.Sleep(2 * time.Second)
	}


	t.Log("Waiting for goroutines to finish")

	elapsed1 := time.Now().Sub(start)

	t.Log(elapsed1.Seconds())

	wg.Wait()

	elapsed := time.Now().Sub(start)

	t.Log(elapsed.Seconds())
	t.Log(elapsed.Nanoseconds())

}

func TestAccountCreation(t *testing.T) {

	rootNode := fmt.Sprintf("WalletA.txt")
	rootNodePubKey, _ := crypto.ExtractECDSAPublicKeyFromFile(rootNode)
	rootNodePrivKey, _ := crypto.ExtractECDSAKeyFromFile(rootNode)
	rootNodeAddress := crypto.GetAddressFromPubKey(rootNodePubKey)
	hasherRootNode := protocol.SerializeHashContent(rootNodeAddress)

	fromPrivKey, _ := crypto.ExtractECDSAKeyFromFile(rootNode)
	var nodeList [][32]byte

	//create 10 new accounts
	for i := 1; i <= 20; i++ {

		accTx, newAccAddress, err := protocol.ConstrAccTx(
			byte(0),
			uint64(1),
			[64]byte{},
			rootNodePrivKey,
			nil,
			nil)

		if err != nil {
			t.Log("got an issue")
		}

		if err := SendTx("127.0.0.1:8000", accTx, p2p.ACCTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}
		if err := SendTx("127.0.0.1:8001", accTx, p2p.ACCTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}

		newNodeAddress := crypto.GetAddressFromPubKey(&newAccAddress.PublicKey)
		hasherNewNode := protocol.SerializeHashContent(newNodeAddress)

		//append all 20 hashers to a new list
		nodeList = append(nodeList, hasherNewNode)

		t.Log(hasherNewNode)
		time.Sleep(10 * time.Second)

	}

	//time.Sleep(200 * time.Second)

	i := 1

	for _, hasherNewNode := range nodeList {


		//send funds to new account
		tx, _ := protocol.ConstrFundsTx(
			byte(0),
			uint64(1000000),
			uint64(1),
			uint32(i),
			hasherRootNode,
			hasherNewNode,
			fromPrivKey,
			fromPrivKey,
			nil)

		if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}
		if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
			fmt.Sprintf("Error")
		}

		i += 1

		time.Sleep(2 * time.Second)

	}


	time.Sleep(60 * time.Second)

	var wg sync.WaitGroup



	wg.Add(len(nodeList))

	start := time.Now()

	for _, hasher := range nodeList {
		go func([32]byte, *sync.WaitGroup) {
			defer wg.Done()
			t.Log(hasher[0:8])
			//now send tx back to the root account. Note that I never did anything manually with the new account
			for txCount := 0; txCount < 10000; txCount++ {
				tx, _ := protocol.ConstrFundsTx(
					byte(0),
					uint64(10),
					uint64(1),
					uint32(txCount),
					hasher,
					hasherRootNode,
					fromPrivKey,
					fromPrivKey,
					nil)

				if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
					t.Log(fmt.Sprintf("Error"))
				}
				if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
					t.Log(fmt.Sprintf("Error"))
				}
			}
			t.Log("One is finished")
		}(hasher, &wg)
		time.Sleep(2 * time.Second)
	}


	t.Log("Waiting for goroutines to finish")

	elapsed1 := time.Now().Sub(start)

	t.Log(elapsed1.Seconds())

	wg.Wait()

	elapsed := time.Now().Sub(start)

	t.Log(elapsed.Seconds())
	t.Log(elapsed.Nanoseconds())

}


func TestOneSender(t *testing.T) {
	rootNode := fmt.Sprintf("WalletA.txt")
	firstNode := fmt.Sprintf("WalletB.txt")

	rootNodePubKey, _ := crypto.ExtractECDSAPublicKeyFromFile(rootNode)
	firstNodePubKey, _ := crypto.ExtractECDSAPublicKeyFromFile(firstNode)

	rootNodeAddress := crypto.GetAddressFromPubKey(rootNodePubKey)
	firstNodeAddress := crypto.GetAddressFromPubKey(firstNodePubKey)


	hasherRootNode := protocol.SerializeHashContent(rootNodeAddress)
	hasherFirstNode := protocol.SerializeHashContent(firstNodeAddress)

	t.Log(hasherRootNode)
	t.Log(hasherFirstNode)

	fromPrivKey, _ := crypto.ExtractECDSAKeyFromFile(rootNode)

	time.Sleep(5 * time.Second)

	// send a lot of funds to the second node
	tx,_ := protocol.ConstrFundsTx(
		byte(0),
		uint64(1000000),
		uint64(1),
		uint32(1),
		hasherRootNode,
		hasherFirstNode,
		fromPrivKey,
		fromPrivKey,
		nil)

	if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
		fmt.Sprintf("Error")
	}
	if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
		fmt.Sprintf("Error")
	}

	time.Sleep(60 * time.Second)

	tx,_ = protocol.ConstrFundsTx(
		byte(0),
		uint64(500000),
		uint64(1),
		uint32(0),
		hasherFirstNode,
		hasherRootNode,
		fromPrivKey,
		fromPrivKey,
		nil)


	if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
		fmt.Sprintf("Error")
	}
	if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
		fmt.Sprintf("Error")
	}

	time.Sleep(100 * time.Second)

	go func() {
		txCount := 1
		for {
			tx, _ := protocol.ConstrFundsTx(
				byte(0),
				uint64(10),
				uint64(1),
				uint32(txCount),
				hasherFirstNode,
				hasherRootNode,
				fromPrivKey,
				fromPrivKey,
				nil)
			if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
				fmt.Sprintf("Error")
				continue
			}
			if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
				fmt.Sprintf("Error")
				continue
			}
			txCount += 1
		}
	}()
	go func() {
		txCount := 2
		for {
			tx, _ := protocol.ConstrFundsTx(
				byte(0),
				uint64(10),
				uint64(1),
				uint32(txCount),
				hasherRootNode,
				hasherFirstNode,
				fromPrivKey,
				fromPrivKey,
				nil)
			if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
				fmt.Sprintf("Error")
				continue
			}
			if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
				fmt.Sprintf("Error")
				continue
			}
			txCount += 1
		}
	}()


	time.Sleep(45 * time.Second)
	t.Log("Done!!!")


}

func TestShardingWith20Nodes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestShardingWith20Nodes...")
	}
	/**
	Set Total Number of desired nodes. They will be generated automatically. And for each node, a separate go routine is being created.
	This enables parallel issuance of transactions
	 */
	TotalNodes = 20

	//Generate wallet directories for all nodes, i.e., validators and non-validators
	for i := 1; i <= TotalNodes; i++ {
		strNode := fmt.Sprintf("Node_%d",i)
		if(!stringAlreadyInSlice(NodeNames,strNode)){
			NodeNames = append(NodeNames,strNode)
		}
		if _, err := os.Stat(NodesDirectory+strNode); os.IsNotExist(err) {
			err = os.MkdirAll(NodesDirectory+strNode, 0755)
			if err != nil {
				t.Errorf("Error while creating node directory %v\n",err)
			}
		}
		storage.Init(NodesDirectory+strNode+"storage.db", TestIpPort)
		_, err := crypto.ExtractECDSAPublicKeyFromFile(NodesDirectory+strNode+"wallet.txt")
		if err != nil {
			return
		}
		_, err = crypto.ExtractRSAKeyFromFile(NodesDirectory+strNode+"commitment.txt")
		if err != nil {
			return
		}
	}
	//
	time.Sleep(30*time.Second)
	//var wg sync.WaitGroup

	transferFundsToWallets()

	time.Sleep(20*time.Second)

	//Create a goroutine for each wallet and send TX from corresponding wallet to root account
	for i := 1; i <= TotalNodes; i++ {
		strNode := fmt.Sprintf("Node_%d",i)
		//wg.Add(1)
		go func() {
			txCount := 0
			//for i := 1; i <= 500; i++ {
			for {
				fromPrivKey, err := crypto.ExtractECDSAKeyFromFile(NodesDirectory+strNode+"wallet.txt")
				if err != nil {
					return
				}

				toPubKey, err := crypto.ExtractECDSAPublicKeyFromFile("WalletA.txt")
				if err != nil {
					return
				}

				fromAddress := crypto.GetAddressFromPubKey(&fromPrivKey.PublicKey)
				//t.Logf("fromAddress: (%x)\n",fromAddress[0:8])
				toAddress := crypto.GetAddressFromPubKey(toPubKey)
				//t.Logf("toAddress: (%x)\n",toAddress[0:8])

				tx, err := protocol.ConstrFundsTx(
					byte(0),
					uint64(10),
					uint64(1),
					uint32(txCount),
					protocol.SerializeHashContent(fromAddress),
					protocol.SerializeHashContent(toAddress),
					fromPrivKey,
					fromPrivKey,
					nil)

				if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
					t.Log(err)
					continue
				}
				if err := SendTx("127.0.0.1:8001", tx, p2p.FUNDSTX_BRDCST); err != nil {
					t.Log(err)
					continue
				}
				//if err := SendTx("127.0.0.1:8002", tx, p2p.FUNDSTX_BRDCST); err != nil {
				//	continue
				//}
				txCount += 1
			}
			//wg.Done()
		}()
	}

	time.Sleep(45*time.Second)

	//wg.Wait()
	t.Log("Done...")
}

func TestSendingFundsTo20Nodes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestSendingFundsTo20Nodes...")
	}
	TotalNodes = 20
	transferFundsToWallets()
}

func transferFundsToWallets() {
	//Transfer 1 Mio. funds to all wallets from root account
	txCountRootAccBeginning := 0
	for i := 1; i <= TotalNodes; i++ {
		strNode := fmt.Sprintf("Node_%d",i)
		fromPrivKey, err := crypto.ExtractECDSAKeyFromFile("walletMinerA.key")
		if err != nil {
			return
		}

		toPubKey, err := crypto.ExtractECDSAPublicKeyFromFile(NodesDirectory+strNode+"/wallet.key")
		if err != nil {
			return
		}

		fromAddress := crypto.GetAddressFromPubKey(&fromPrivKey.PublicKey)
		toAddress := crypto.GetAddressFromPubKey(toPubKey)

		tx, err := protocol.ConstrFundsTx(
			byte(0),
			uint64(10000),
			uint64(1),
			uint32(txCountRootAccBeginning),
			protocol.SerializeHashContent(fromAddress),
			protocol.SerializeHashContent(toAddress),
			fromPrivKey,
			fromPrivKey,
			nil)

		if err := SendTx("127.0.0.1:8000", tx, p2p.FUNDSTX_BRDCST); err != nil {
			return
		}
		txCountRootAccBeginning += 1
	}
}

func SendTx(dial string, tx protocol.Transaction, typeID uint8) (err error) {
	if conn := p2p.Connect(dial); conn != nil {
		packet := p2p.BuildPacket(typeID, tx.Encode())
		conn.Write(packet)

		header, payload, err := p2p.RcvData_(conn)
		if err != nil || header.TypeID == p2p.NOT_FOUND {
			err = errors.New(string(payload[:]))
		}
		conn.Close()

		return err
	}

	txHash := tx.Hash()
	return errors.New(fmt.Sprintf("Sending tx %x failed.", txHash[:8]))
}


func stringAlreadyInSlice(inputSlice []string, str string) bool {
	for _, entry := range inputSlice {
		if entry == str {
			return true
		}
	}
	return false
}