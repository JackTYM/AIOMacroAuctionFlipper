package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Auction struct {
	UUID             string   `json:"uuid"`
	Auctioneer       string   `json:"auctioneer"`
	ProfileId        string   `json:"profile_id"`
	Coop             []string `json:"coop"`
	Start            int64    `json:"start"`
	End              int64    `json:"end"`
	ItemName         string   `json:"item_name"`
	ItemLore         string   `json:"item_lore"`
	Extra            string   `json:"extra"`
	Category         string   `json:"category"`
	StartingBid      int64    `json:"starting_bid"`
	ItemBytes        string   `json:"item_bytes"`
	Claimed          bool     `json:"claimed"`
	ClaimedBidders   []string `json:"claimed_bidders"`
	HighestBidAmount int64    `json:"highest_bid_amount"`
	LastUpdated      int64    `json:"last_updated"`
	Bin              bool     `json:"bin"`
	Bids             []Bid    `json:"bids"`
	ItemUUID         string   `json:"item_uuid"`
}

type Bid struct {
	AuctionId string `json:"auction_id"`
	Bidder    string `json:"bidder"`
	ProfileId string `json:"profile_id"`
	Amount    string `json:"amount"`
	Timestamp int64  `json:"timestamp"`
}

type AuctionResponse struct {
	Success       bool      `json:"success"`
	Page          int       `json:"page"`
	TotalPages    int       `json:"totalPages"`
	TotalAuctions int       `json:"totalAuctions"`
	LastUpdated   int64     `json:"lastUpdated"`
	Auctions      []Auction `json:"auctions"`
}
type AuctionValue struct {
	UUID                  string `json:"uuid"`
	ItemName              string `json:"item_name"`
	InternalId            string `json:"internal_id"`
	StartingBid           int64  `json:"starting_bid"`
	LowestBIN             int64  `json:"lowest_bin"`
	Profit                int64  `json:"profit"`
	PotentialManipulation bool   `json:"potential_manipulation"`
}

var lastUpdated int64 = 0
var globalTest bool
var globalAuctions []Auction
var lowestBins map[string]interface{}
var averageLowestBins map[string]interface{}
var lowestBinToItemName map[string]string
var itemNameToLowestBinName map[string]string
var reforgeNames = [27]string{"Gentle", "Odd", "Fast", "Fair", "Epic", "Sharp", "Heroic", "Spicy", "Legendary", "Dirty", "Fabled", "Suspicious", "Gilded", "Warped", "Withered", "Bulky", "Salty", "Treacherous", "Stiff", "Lucky", "Very", "Highly", "Extremely", "Not So", "Thicc", "Absolutely", "Even More"}

// Main function
func main() {

	jsonFile, err := os.Open("items.json")
	// Open our jsonFile
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
	}
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var result map[string]interface{}
	json.Unmarshal(byteValue, &result)
	lowestBinToItemName = make(map[string]string)
	itemNameToLowestBinName = make(map[string]string)

	for key, value := range result {
		item := value.(map[string]interface{})
		name := item["name"].(string)
		lowestBinToItemName[key] = name
		itemNameToLowestBinName[name] = key
	}

	// defer the closing of our jsonFile so that we can parse it later on
	defer jsonFile.Close()
	lowestBins = callLowestBin()
	averageLowestBins = callAverageLowestBin()

	// Start the item grab loop
	go dataGrabLoop()
	fmt.Println("Grabbing update data")

	// Init the mux router
	router := mux.NewRouter()

	// Route handles & endpoints

	// Get all items
	router.HandleFunc("/items", getClientItems).Methods("GET")

	// serve the app
	fmt.Println("Server at 8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}

// Grab auction data every 60 seconds
func dataGrabLoop() {
	if lastUpdated == 0 {
		// get the first page to get the lastUpdatedTime
		responseObject := getHypixelPage(0)
		lastUpdated = responseObject.LastUpdated
	}

	wg := &sync.WaitGroup{}
	for true {
		if !globalTest {
			wg.Add(1)
			go checkReady(wg, time.Now().UnixMilli() >= lastUpdated+60000)
		} else {
			if lastUpdated != getHypixelPage(0).LastUpdated {
				globalTest = false
				break
			}
		}
	}

	fmt.Printf("Grab Success! Refreshed Api in %d\n", time.Now().UnixMilli()-getHypixelPage(0).LastUpdated)
	globalAuctions = callHypixelAuctions(lastUpdated)
	lastUpdated = 0

	go dataGrabLoop()
	return
}

func checkReady(wg *sync.WaitGroup, valueToWait bool) {
	if valueToWait {
		globalTest = true
	}
	wg.Done()
	return
}

/*
// send to webhook
func parseItems() {
	var uuidList []string

	// filter bin == true, calc value, sort by value
	for _, a := range globalAuctions {
		parse := true
		for _, u := range uuidList {
			if u == a.UUID {
				parse = false
			}
		}
		if a.Bin && parse {
			lbName := inGameToApi(a.ItemName, a.ItemLore)
			if lowestBins[lbName] != nil {
				priceFloat, _ := lowestBins[lbName].(json.Number).Float64()
				AveragePriceFloat, _ := averageLowestBins[lbName].(json.Number).Float64()
				lowestBin := int64(priceFloat)
				averageLowestBin := int64(AveragePriceFloat)
				var potentialManipulation bool
				averageIncrease := 1000 - (1000 * (lowestBin * 1000) / (averageLowestBin * 1000))
				if averageIncrease < 0 {
					if averageIncrease <= -1000 {
						potentialManipulation = true
					} else {
						potentialManipulation = false
					}
				} else {
					if averageIncrease >= 1000 {
						potentialManipulation = true
					} else {
						potentialManipulation = false
					}
				}
				if lowestBin > 0 {
					av := AuctionValue{
						UUID:                  a.UUID,
						ItemName:              a.ItemName,
						InternalId:            lbName,
						StartingBid:           a.StartingBid,
						LowestBIN:             lowestBin,
						Profit:                lowestBin - a.StartingBid,
						PotentialManipulation: potentialManipulation,
					}
					//if av.Profit >= 500000 {

					message := fmt.Sprintf("{\n  \"content\": null,\n  \"embeds\": [\n    {\n      \"title\": \"Auction Flip Detected\",\n      \"description\": \"Item: %d\\nProfit: %d\\nPrice: %d\\nLowest BIN: %d\\nPotential Manipulation: %d\\nAuction Id: %d\",\n      \"color\": 1625924\n    }\n  ],\n  \"attachments\": []\n}", av.ItemName, av.Profit, av.StartingBid, av.LowestBIN, av.PotentialManipulation, av.UUID)

					jsonBytes, err := json.Marshal(message)

					post, err := http.Post("https://discord.com/api/webhooks/987961746566287390/2WOulebkyN53jqe3ZpqyQsz0IYEbFa_dTu6KrpDgYEX5bh6SNuKkwHUHr6F2qVQ6YLya", "application/json", bytes.NewBuffer(jsonBytes))

					fmt.Println(err)
					fmt.Println(post)

					//}
				}
			}
		}
	}
}*/

// response and request handlers
func getClientItems(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Handler for /items")

	var uuidList []string

	var minProfit int64
	minProfit, _ = strconv.ParseInt(r.FormValue("lowest_profit"), 10, 64)
	var enchantments bool
	enchantments, _ = strconv.ParseBool(r.FormValue("enchantments"))
	var petSkins bool
	petSkins, _ = strconv.ParseBool(r.FormValue("pet_skins"))
	var manipulation bool
	manipulation, _ = strconv.ParseBool(r.FormValue("manipulation"))
	auctionValues := make([]AuctionValue, 0)

	// filter bin == true, calc value, sort by value
	for _, a := range globalAuctions {
		parse := true
		for _, u := range uuidList {
			if u == a.UUID {
				parse = false
			}
		}
		if a.Bin && parse {
			lbName := inGameToApi(a.ItemName, a.ItemLore)
			if lowestBins[lbName] != nil {
				priceFloat, _ := lowestBins[lbName].(json.Number).Float64()
				AveragePriceFloat, _ := averageLowestBins[lbName].(json.Number).Float64()
				lowestBin := int64(priceFloat)
				averageLowestBin := int64(AveragePriceFloat)
				var potentialManipulation bool
				averageIncrease := 1000 - (1000 * (lowestBin * 1000) / (averageLowestBin * 1000))
				if averageIncrease < 0 {
					if averageIncrease <= -1000 {
						potentialManipulation = true
					} else {
						potentialManipulation = false
					}
				} else {
					if averageIncrease >= 1000 {
						potentialManipulation = true
					} else {
						potentialManipulation = false
					}
				}
				if lowestBin > 0 {
					av := AuctionValue{
						UUID:                  a.UUID,
						ItemName:              a.ItemName,
						InternalId:            lbName,
						StartingBid:           a.StartingBid,
						LowestBIN:             lowestBin,
						Profit:                lowestBin - a.StartingBid,
						PotentialManipulation: potentialManipulation,
					}
					if av.Profit > minProfit && (enchantments || (!enchantments && a.ItemName != "Enchanted Book") && (petSkins || (!petSkins && !strings.Contains(a.ItemName, "Skin"))) && (manipulation || (!manipulation && !av.PotentialManipulation))) {
						uuidList = append(uuidList, a.UUID)
						auctionValues = append(auctionValues, av)
					} else {
						//fmt.Printf("Skipping %s with starting bid %d and lowestBin %d\n", av.ItemName, av.StartingBid, lowestBin)
					}
				}
			}
		}
	}
	sort.Slice(auctionValues, func(i, j int) bool {
		return auctionValues[i].Profit > auctionValues[j].Profit
	})

	json.NewEncoder(w).Encode(auctionValues)
}

func callHypixelAuctions(lastRunTime int64) []Auction {
	limiter := make(chan bool, 10)

	var auctionList [][]Auction
	wg := sync.WaitGroup{}

	var auctions []Auction

	// get the first page to get the page count
	responseObject := getHypixelPage(0)
	totalPages := responseObject.TotalPages

	fmt.Printf("Calling HypixelAuctions to get all items updated more recently than %d\n", lastRunTime)
	for i := 1; i < totalPages; i++ {
		wg.Add(1)
		go func(page int) {
			limiter <- true
			defer func() {
				<-limiter
				wg.Done()
			}()
			fmt.Printf("Calling for page %d\n", page)
			responseObject := getHypixelPage(page)
			if len(responseObject.Auctions) > 0 {
				auctionList = append(auctionList, responseObject.Auctions)
			}
		}(i)
	}
	wg.Wait()
	for _, element := range auctionList {
		auctions = append(auctions, element...)
	}
	return auctions
}

func getHypixelPage(page int) AuctionResponse {
	response, err := http.Get(fmt.Sprintf("https://api.hypixel.net/skyblock/auctions?page=%d", page))

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	var responseObject AuctionResponse
	json.Unmarshal(responseData, &responseObject)
	return responseObject
}

func callLowestBin() map[string]interface{} {
	response, err := http.Get("https://moulberry.codes/lowestbin.json")
	fmt.Println("Calling Lowest Bin to get value for items")

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	var itemPrices map[string]interface{}

	d := json.NewDecoder(bytes.NewReader(responseData))
	d.UseNumber()

	d.Decode(&itemPrices)

	return itemPrices
}

func callAverageLowestBin() map[string]interface{} {
	response, err := http.Get("https://moulberry.codes/auction_averages_lbin/3day.json")
	fmt.Println("Calling Average Lowest Bin to get value for items")

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	var itemPrices map[string]interface{}

	d := json.NewDecoder(bytes.NewReader(responseData))
	d.UseNumber()

	d.Decode(&itemPrices)

	return itemPrices
}

func normalizeName(name string) string {
	// remove all non-ascii characters
	name = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, name)
	// remove all reforge names
	for _, f := range reforgeNames {
		name = strings.ReplaceAll(name, f, "")
	}
	return name
}

func inGameToApi(inGame string, lore string) string {
	var api string
	inGame = normalizeName(inGame)

	switch inGame {
	case "Armor Stand":
		api = "ARMOR_SHOWCASE"
	case "Beastmaster Crest":
		if strings.Contains(lore, "UNCOMMON") || (strings.Contains(lore, "RARE") && strings.Contains(lore, "§k")) {
			api = "BEASTMASTER_CREST_UNCOMMON"
		} else if strings.Contains(lore, "COMMON") || (strings.Contains(lore, "UNCOMMON") && strings.Contains(lore, "§k")) {
			api = "BEASTMASTER_CREST_COMMON"
		} else if strings.Contains(lore, "RARE") || (strings.Contains(lore, "EPIC") && strings.Contains(lore, "§k")) {
			api = "BEASTMASTER_CREST_RARE"
		} else if strings.Contains(lore, "EPIC") || (strings.Contains(lore, "LEGENDARY") && strings.Contains(lore, "§k")) {
			api = "BEASTMASTER_CREST_EPIC"
		} else if strings.Contains(lore, "LEGENDARY") || (strings.Contains(lore, "MYTHIC") && strings.Contains(lore, "§k")) {
			api = "BEASTMASTER_CREST_LEGENDARY"
		}
	case "Cauldron":
		api = "CAULDRON"
	case "Combat Exp Boost":
		if strings.Contains(lore, "UNCOMMON") {
			api = "PET_ITEM_COMBAT_SKILL_BOOST_UNCOMMON"
		} else if strings.Contains(lore, "COMMON") {
			api = "PET_ITEM_COMBAT_SKILL_BOOST_COMMON"
		} else if strings.Contains(lore, "RARE") {
			api = "PET_ITEM_COMBAT_SKILL_BOOST_RARE"
		} else if strings.Contains(lore, "EPIC") {
			api = "PET_ITEM_COMBAT_SKILL_BOOST_EPIC"
		}
	case "Enchanted Book Bundle":
		if strings.Contains(lore, "Big Brain 3") {
			api = "ENCHANTED_BOOK_BUNDLE_BIG_BRAIN"
		} else if strings.Contains(lore, "Vicious 3") {
			api = "ENCHANTED_BOOK_BUNDLE_VICIOUS"
		}
	case "Farming Exp Boost":
		if strings.Contains(lore, "UNCOMMON") {
			api = "PET_ITEM_FARMING_SKILL_BOOST_UNCOMMON"
		} else if strings.Contains(lore, "COMMON") {
			api = "PET_ITEM_FARMING_SKILL_BOOST_COMMON"
		} else if strings.Contains(lore, "RARE") {
			api = "PET_ITEM_FARMING_SKILL_BOOST_RARE"
		} else if strings.Contains(lore, "EPIC") {
			api = "PET_ITEM_FARMING_SKILL_BOOST_EPIC"
		}
	case "Fishing Exp Boost":
		if strings.Contains(lore, "UNCOMMON") {
			api = "PET_ITEM_FISHING_SKILL_BOOST_UNCOMMON"
		} else if strings.Contains(lore, "COMMON") {
			api = "PET_ITEM_FISHING_SKILL_BOOST_COMMON"
		} else if strings.Contains(lore, "RARE") {
			api = "PET_ITEM_FISHING_SKILL_BOOST_RARE"
		} else if strings.Contains(lore, "EPIC") {
			api = "PET_ITEM_FISHING_SKILL_BOOST_EPIC"
		}
	case "Flamebreaker Helmet":
		api = "FLAME_BREAKER_HELMET"
	case "Flamebreaker Chestplate":
		api = "FLAME_BREAKER_CHESTPLATE"
	case "Flamebreaker Leggings":
		api = "FLAME_BREAKER_LEGGINGS"
	case "Flamebreaker Boots":
		api = "FLAME_BREAKER_BOOTS"
	case "Flower Pot":
		api = "FANCY_FLOWER_POT"
	case "Foraging Exp Boost":
		if strings.Contains(lore, "COMMON") {
			api = "PET_ITEM_FORAGING_SKILL_BOOST_COMMON"
		} else if strings.Contains(lore, "EPIC") {
			api = "PET_ITEM_FORAGING_SKILL_BOOST_EPIC"
		}
	case "God Potion":
		if strings.Contains(lore, "Legacy") {
			api = "GOD_POTION"
		} else if strings.Contains(lore, "EPIC") {
			api = "GOD_POTION_2"
		}
	case "Griffin Upgrade Stone":
		if strings.Contains(lore, "COMMON") {
			api = "GRIFFIN_UPGRADE_STONE_UNCOMMON"
		} else if strings.Contains(lore, "UNCOMMON") {
			api = "GRIFFIN_UPGRADE_STONE_UNCOMMON"
		} else if strings.Contains(lore, "RARE") {
			api = "GRIFFIN_UPGRADE_STONE_RARE"
		} else if strings.Contains(lore, "EPIC") {
			api = "GRIFFIN_UPGRADE_STONE_EPIC"
		} else if strings.Contains(lore, "LEGENDARY") {
			api = "GRIFFIN_UPGRADE_STONE_LEGENDARY"
		}
	case "Helmet of the Rising Sun":
		api = "ARMOR_OF_THE_RESISTANCE_HELMET"
	case "Chestplate of the Rising Sun":
		api = "ARMOR_OF_THE_RESISTANCE_CHESTPLATE"
	case "Leggings of the Rising Sun":
		api = "ARMOR_OF_THE_RESISTANCE_LEGGINGS"
	case "Boots of the Rising Sun":
		api = "ARMOR_OF_THE_RESISTANCE_BOOTS"
	case "Staff of the Rising Sun":
		api = "HOPE_OF_THE_RESISTANCE"
	case "Mining Exp Boost":
		if strings.Contains(lore, "COMMON") {
			api = "PET_ITEM_MINING_SKILL_BOOST_COMMON"
		} else if strings.Contains(lore, "RARE") {
			api = "PET_ITEM_MINING_SKILL_BOOST_RARE"
		}
	case "Saddle":
		api = "PET_ITEM_SADDLE"
	case "Salmon Helmet":
		if strings.Contains(lore, "RARE") {
			api = "SALMON_HELMET_NEW"
		} else {
			api = "SALMON_HELMET"
		}
	case "Salmon Chestplate":
		if strings.Contains(lore, "RARE") {
			api = "SALMON_CHESTPLATE_NEW"
		} else {
			api = "SALMON_CHESTPLATE"
		}
	case "Salmon Leggings":
		if strings.Contains(lore, "RARE") {
			api = "SALMON_LEGGINGS_NEW"
		} else {
			api = "SALMON_LEGGINGS"
		}
	case "Salmon Boots":
		if strings.Contains(lore, "RARE") {
			api = "SALMON_BOOTS_NEW"
		} else {
			api = "SALMON_BOOTS"
		}
	case "Shimmer Skin":
		if strings.Contains(lore, "Superior") {
			api = "SUPERIOR_SHIMMER"
		} else if strings.Contains(lore, "Strong") {
			api = "STRONG_SHIMMER"
		} else if strings.Contains(lore, "Unstable") {
			api = "UNSTABLE_SHIMMER"
		} else if strings.Contains(lore, "Young") {
			api = "YOUNG_SHIMMER"
		} else if strings.Contains(lore, "Wise") {
			api = "WISE_SHIMMER"
		} else if strings.Contains(lore, "Holy") {
			api = "HOLY_SHIMMER"
		} else if strings.Contains(lore, "Old") {
			api = "OLD_SHIMMER"
		} else if strings.Contains(lore, "Protector") {
			api = "PROTECTOR_SHIMMER"
		}
	case "Baby Skin":
		if strings.Contains(lore, "Superior") {
			api = "SUPERIOR_BABY"
		} else if strings.Contains(lore, "Strong") {
			api = "STRONG_BABY"
		} else if strings.Contains(lore, "Unstable") {
			api = "UNSTABLE_BABY"
		} else if strings.Contains(lore, "Young") {
			api = "YOUNG_BABY"
		} else if strings.Contains(lore, "Wise") {
			api = "WISE_BABY"
		} else if strings.Contains(lore, "Holy") {
			api = "HOLY_BABY"
		} else if strings.Contains(lore, "Old") {
			api = "OLD_BABY"
		} else if strings.Contains(lore, "Protector") {
			api = "PROTECTOR_BABY"
		}
	case "Silex":
		api = "SIL_EX"
	case "Small Backpack":
		api = "SMALL_BACKPACK"
	case "Spirit Bow":
		api = "ITEM_SPIRIT_BOW"
	case "Enchanted Book":
		m1 := regexp.MustCompile("(?i)\u00a7[0-9a-fk-or]")
		enchant := m1.ReplaceAllString(strings.Split(lore, "\n")[0], "")
		if strings.Contains(enchant, ",") {
			enchant = strings.Split(enchant, ",")[0]
		}
		var enchantNum string
		switch strings.Split(enchant, " ")[len(strings.Split(enchant, " "))-1] {
		case "I":
			enchantNum = "1"
		case "II":
			enchantNum = "2"
		case "III":
			enchantNum = "3"
		case "IV":
			enchantNum = "4"
		case "V":
			enchantNum = "5"
		case "VI":
			enchantNum = "6"
		case "VII":
			enchantNum = "7"
		case "VIII":
			enchantNum = "8"
		case "IX":
			enchantNum = "9"
		case "X":
			enchantNum = "10"
		default:
			enchantNum = "0"
		}
		enchant = strings.ReplaceAll(enchant, " "+strings.Split(enchant, " ")[len(strings.Split(enchant, " "))-1], "")
		enchant = strings.ToUpper(strings.ReplaceAll(enchant, " ", "_") + ";" + enchantNum)
		api = enchant
	default:
		api = itemNameToLowestBinName[inGame]
	}

	return api
}
