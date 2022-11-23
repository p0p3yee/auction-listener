package main

import (
	"auction/x/auction/types"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ignite/cli/ignite/pkg/cosmosclient"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
)

const Addressprefix = "auction"
const BlockchainApiEndpoint = "http://127.0.0.1:1317"

type TxBody struct {
	Messages []map[string]string `json:"messages"`
}

type Tx struct {
	Body TxBody `json:"body"`
}

type BlockTxResult struct {
	Tx Tx `json:"tx"`
}

func getTx(txid string) (*BlockTxResult, error) {
	url := fmt.Sprintf(
		"%s/cosmos/tx/v1beta1/txs/%s",
		BlockchainApiEndpoint,
		txid,
	)

	resp, err := http.Get(url)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var blockTxResult BlockTxResult
	err = json.Unmarshal(body, &blockTxResult)
	if err != nil {
		return nil, err
	}
	return &blockTxResult, nil
}

func main() {

	cosmos, err := cosmosclient.New(
		context.Background(),
		cosmosclient.WithAddressPrefix(Addressprefix),
	)
	if err != nil {
		log.Fatal(err)
	}

	queryClient := types.NewQueryClient(cosmos.Context())

	resp, err := queryClient.Auctions(context.Background(), &types.QueryAuctionsRequest{})
	if err != nil {
		log.Fatal(err)
	}

	bids_resp, err := queryClient.Bids(context.Background(), &types.QueryBidsRequest{})
	if err != nil {
		log.Fatal(err)
	}

	allAuctions := resp.GetAuction()
	allBids := bids_resp.GetBid()

	currentMaxBid := make(map[uint64]types.Bid)
	auctionMap := make(map[uint64]types.Auction)
	auctionMaxId := uint64(len(allAuctions))

	for _, auction := range allAuctions {
		auctionMap[auction.Id] = *auction
		if auction.HighestBidExists {
			currentMaxBid[auction.Id] = *allBids[auction.CurrentHighestBidId]
		}
	}

	log.Printf("\nTotal Auction Now: %d", auctionMaxId)

	for {
		cosmos.WaitForNextBlock(context.Background())

		height, err := cosmos.LatestBlockHeight(context.Background())
		if err != nil {
			log.Fatal(err)
		}

		txs, err := cosmos.GetBlockTXs(context.Background(), height)
		if err != nil {
			log.Fatal(err)
		}

		for _, tx := range txs {
			if tx.Raw.TxResult.Code != 0 {
				continue
			}

			resp, err := getTx(tx.Raw.Hash.String())
			if err != nil {
				log.Fatal(err)
			}

			for _, msg := range resp.Tx.Body.Messages {
				if msg["@type"] == "/auction.auction.MsgPlaceBid" {
					log.Printf(
						"[%d] [%s] | Placed bid on Auction id: %s | Bid Price: %s |\n",
						height,
						msg["creator"],
						msg["auctionId"],
						msg["bidPrice"],
					)

					auctionId, err := strconv.ParseUint(msg["auctionId"], 10, 64)
					if err != nil {
						log.Fatal(err)
					}

					currentMaxBid[auctionId] = types.Bid{
						Creator:   msg["creator"],
						Id:        0,
						AuctionId: auctionId,
						BidPrice:  msg["bidPrice"],
					}

				} else if msg["@type"] == "/auction.auction.MsgCreateAuction" {
					log.Printf(
						"[%d] [%s] | Created Auction | Name: %s | Start Price: %s | Duration: %s |\n",
						height,
						msg["creator"],
						msg["name"],
						msg["startPrice"],
						msg["duration"],
					)

					duration, err := strconv.ParseUint(msg["duration"], 10, 64)
					if err != nil {
						log.Fatal(err)
					}

					auctionMap[auctionMaxId] = types.Auction{
						Creator:    msg["creator"],
						Id:         auctionMaxId,
						Name:       msg["name"],
						StartPrice: msg["startPrice"],
						Duration:   duration,
						Ended:      false,
					}
					auctionMaxId++
				} else if msg["@type"] == "/auction.auction.MsgFinalizeAuction" {
					auctionId, err := strconv.ParseUint(msg["auctionId"], 10, 64)
					if err != nil {
						log.Fatal(err)
					}

					log.Printf(
						"[%d] [%s] | Auction Finalized | Name: %s | Start Price: %s | Final Price: %s | Winner: %s | \n",
						height,
						msg["creator"],
						auctionMap[auctionId].Name,
						auctionMap[auctionId].StartPrice,
						currentMaxBid[auctionId].BidPrice,
						currentMaxBid[auctionId].Creator,
					)

					delete(currentMaxBid, auctionId)
				}
			}
		}

		if height%100 == 0 {
			for auctionId, highestBid := range currentMaxBid {
				log.Printf(
					"[%d] | Current Highest Bid for Auction ID: %d is %s By: %s",
					height,
					auctionId,
					highestBid.BidPrice,
					highestBid.Creator,
				)
			}
		}
	}

}
