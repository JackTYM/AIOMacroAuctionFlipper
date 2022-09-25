package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/gorilla/mux"
	"golang.org/x/exp/slices"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Auction struct {
	UUID             string        `json:"uuid"`
	Auctioneer       string        `json:"auctioneer"`
	ProfileID        string        `json:"profile_id"`
	Coop             []string      `json:"coop"`
	Start            int64         `json:"start"`
	End              int64         `json:"end"`
	ItemName         string        `json:"item_name"`
	ItemLore         string        `json:"item_lore"`
	Extra            string        `json:"extra"`
	Category         string        `json:"category"`
	Tier             string        `json:"tier"`
	StartingBid      int           `json:"starting_bid"`
	ItemBytes        string        `json:"item_bytes"`
	Claimed          bool          `json:"claimed"`
	ClaimedBidders   []interface{} `json:"claimed_bidders"`
	HighestBidAmount int           `json:"highest_bid_amount"`
	LastUpdated      int64         `json:"last_updated"`
	Bin              bool          `json:"bin"`
	Bids             []struct {
		AuctionID string `json:"auction_id"`
		Bidder    string `json:"bidder"`
		ProfileID string `json:"profile_id"`
		Amount    int    `json:"amount"`
		Timestamp int64  `json:"timestamp"`
	} `json:"bids"`
}

type AuctionResponse struct {
	Success       bool      `json:"success"`
	Page          int       `json:"page"`
	TotalPages    int       `json:"totalPages"`
	TotalAuctions int       `json:"totalAuctions"`
	LastUpdated   int64     `json:"lastUpdated"`
	Auctions      []Auction `json:"auctions"`
}

type AuctionFlip struct {
	UUID                  string
	ItemName              string
	Price                 int64
	LowestBIN             int64
	Profit                int64
	PotentialManipulation bool
	TimeAdded             int64
}

var (
	lastUpdated             int64 = 0
	globalFlips             []AuctionFlip
	lowestBins              map[string]interface{}
	averageLowestBins       map[string]interface{}
	lowestBinToItemName     map[string]string
	itemNameToLowestBinName map[string]string
	reforgeNames            = [27]string{"Gentle", "Odd", "Fast", "Fair", "Epic", "Sharp", "Heroic", "Spicy", "Legendary", "Dirty", "Fabled", "Suspicious", "Gilded", "Warped", "Withered", "Bulky", "Salty", "Treacherous", "Stiff", "Lucky", "Very", "Highly", "Extremely", "Not So", "Thicc", "Absolutely", "Even More"}
	uuidList                []string
)

// Main function
func main() {

	jsonFile, err := os.Open("items.json")
	// Open our jsonFile
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 99")
	}
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var result map[string]interface{}
	err = json.Unmarshal(byteValue, &result)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 106")
	}
	lowestBinToItemName = make(map[string]string)
	itemNameToLowestBinName = make(map[string]string)

	for key, value := range result {
		item := value.(map[string]interface{})
		name := item["name"].(string)
		lowestBinToItemName[key] = name
		itemNameToLowestBinName[name] = key
	}

	// defer the closing of our jsonFile so that we can parse it later on
	defer func(jsonFile *os.File) {
		err := jsonFile.Close()
		if err != nil {
			fmt.Println("Error! " + err.Error() + " 112")
		}
	}(jsonFile)
	lowestBins = callLowestBin()
	averageLowestBins = callAverageLowestBin()

	// Start the item grab loop
	go dataGrabLoop()

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
	for true {
		if time.Now().UnixMilli() >= lastUpdated+55000 && lastUpdated != getHypixelPage(0).LastUpdated {
			go callHypixelAuctions(lastUpdated)
			lastUpdated = getHypixelPage(0).LastUpdated
		}
	}
}

// send to webhook
func sendWebhooks(flips []AuctionFlip) {
	for _, a := range flips {
		message := map[string]interface{}{
			"content": nil,
			"embeds": []map[string]interface{}{
				{
					"title": "Auction Flip Detected",
					"fields": []map[string]string{
						{
							"name":   "Item:",
							"value":  a.ItemName,
							"inline": "true",
						},
						{
							"name":   "Profit:",
							"value":  humanize.Comma(a.Profit),
							"inline": "true",
						},
						{
							"name":   "Lowest BIN:",
							"value":  humanize.Comma(a.LowestBIN),
							"inline": "true",
						},
						{
							"name":   "Price:",
							"value":  humanize.Comma(a.Price),
							"inline": "true",
						},
						{
							"name":   "Potential Manipulation:",
							"value":  strconv.FormatBool(a.PotentialManipulation),
							"inline": "true",
						},
						{
							"name":   "Auction Id:",
							"value":  a.UUID,
							"inline": "true",
						},
					},
				},
			},
			"attachments": []string{},
		}

		bytesRepresentation, err := json.Marshal(message)
		if err != nil {
			fmt.Println("Error! " + err.Error() + " 196")
		}

		_, err = http.Post("https://discord.com/api/webhooks/1023043258756124712/GE9HaPfTb11M7K93C5z6i7CXAzTMusP5laXUgvPj5BywjmQd8n132C68HT56caGHke4V", "application/json", bytes.NewBuffer(bytesRepresentation))
		if err != nil {
			fmt.Println("Error! " + err.Error() + " 201")
		}
	}
}

func checkAuctions(auctionList []Auction) {
	var flips []AuctionFlip

	for _, a := range auctionList {
		if !slices.Contains(uuidList, a.UUID) && a.Bin {
			uuidList = append(uuidList, a.UUID)

			lbName := inGameToApi(a.ItemName, a.ItemLore)
			if lowestBins[lbName] != nil {
				priceFloat, _ := lowestBins[lbName].(json.Number).Float64()
				AveragePriceFloat, _ := averageLowestBins[lbName].(json.Number).Float64()
				lowestBin := int64(priceFloat)
				averageLowestBin := int64(AveragePriceFloat)

				if averageLowestBin == 0 {
					averageLowestBin = 1
				}

				averageIncrease := 1000 - (1000 * (lowestBin * 1000) / (averageLowestBin * 1000))

				if lowestBin > 0 {
					if lowestBin-int64(a.StartingBid) >= 100000 {
						flips = append(flips, AuctionFlip{
							UUID:                  a.UUID,
							ItemName:              a.ItemName,
							Price:                 int64(a.StartingBid),
							LowestBIN:             lowestBin,
							Profit:                lowestBin - int64(a.StartingBid),
							PotentialManipulation: averageIncrease <= -1000 || averageIncrease >= 1000,
							TimeAdded:             time.Now().UnixMilli(),
						})
					}
				}
			}
		}
	}

	globalFlips = append(globalFlips, flips...)

	go sendWebhooks(flips)
}

// response and request handlers
func getClientItems(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Handler for /items")

	var minProfit int64
	minProfit, _ = strconv.ParseInt(r.FormValue("min_profit"), 10, 64)
	var maxPrice int64
	maxPrice, _ = strconv.ParseInt(r.FormValue("max_price"), 10, 64)
	var petSkins bool
	petSkins, _ = strconv.ParseBool(r.FormValue("pet_skins"))
	var manipulation bool
	manipulation, _ = strconv.ParseBool(r.FormValue("manipulation"))
	auctionValues := make([]AuctionFlip, 0)

	// filter calc value, sort by value
	for _, a := range globalFlips {
		if time.Now().UnixMilli()-a.TimeAdded <= 100000 && a.Profit > minProfit && (a.Price < maxPrice || maxPrice == 0) && (petSkins || (!petSkins && !strings.Contains(a.ItemName, "Skin"))) && (manipulation || (!manipulation && !a.PotentialManipulation)) {
			fmt.Println("Added item!")
			auctionValues = append(auctionValues, a)
		} else {
			fmt.Println("Ignored item!")
		}
	}
	sort.Slice(auctionValues, func(i, j int) bool {
		return auctionValues[i].Profit > auctionValues[j].Profit
	})

	err := json.NewEncoder(w).Encode(auctionValues)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 291")
	}
}

func callHypixelAuctions(lastRunTime int64) {
	limiter := make(chan bool, 15)

	wg := sync.WaitGroup{}

	// get the first page to get the page count
	totalPages := getHypixelPage(0).TotalPages

	fmt.Printf("Calling Hypixel Auctions to get all items updated more recently than %d\n", lastRunTime)
	for i := 1; i <= totalPages; i++ {
		wg.Add(1)
		go func(page int) {
			limiter <- true
			defer func() {
				<-limiter
				wg.Done()
			}()
			//fmt.Printf("Calling for page %d\n", page)
			responseObject := getHypixelPage(page)
			if len(responseObject.Auctions) > 0 {
				go checkAuctions(responseObject.Auctions)
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("Grab Success! Refreshed Api in %s\n", time.Now().Sub(time.Unix(0, (lastRunTime+60000)*int64(time.Millisecond))).String())
}

func getHypixelPage(page int) AuctionResponse {
	response, err := http.Get(fmt.Sprintf("https://api.hypixel.net/skyblock/auctions?page=%d", page))

	if err != nil {
		fmt.Println("Error! " + err.Error() + " 312")
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 318")
	}

	var responseObject AuctionResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 316")
	}
	return responseObject
}

func callLowestBin() map[string]interface{} {
	response, err := http.Get("https://moulberry.codes/lowestbin.json")
	fmt.Println("Calling Lowest Bin to get value for items")

	if err != nil {
		fmt.Println("Error! " + err.Error() + " 331")
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 337")
	}

	var itemPrices map[string]interface{}

	d := json.NewDecoder(bytes.NewReader(responseData))
	d.UseNumber()

	err = d.Decode(&itemPrices)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 342")
	}

	return itemPrices
}

func callAverageLowestBin() map[string]interface{} {
	response, err := http.Get("https://moulberry.codes/auction_averages_lbin/3day.json")
	fmt.Println("Calling Average Lowest Bin to get value for items")

	if err != nil {
		fmt.Println("Error! " + err.Error() + " 335")
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 361")
	}

	var itemPrices map[string]interface{}

	d := json.NewDecoder(bytes.NewReader(responseData))
	d.UseNumber()

	err = d.Decode(&itemPrices)
	if err != nil {
		fmt.Println("Error! " + err.Error() + " 369")
	}

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
	default:
		api = itemNameToLowestBinName[inGame]
	}

	return api
}
