package http

// auction.cgi okuru is depot.cgi okuru, so the venue reuses the Depot send commands.
var auctionFacility = facility{
	title: "オークション会場", commands: []string{"depot_send_item", "depot_send_money"}, placeLog: true,
	npc: &npcProfile{name: "@ワイルド", words: []string{ // auction.cgi @words
		"ここはオークション会場です。他のプレイヤーとアイテム交換やアイテム売買をする場所です。",
		"入札や出品のようなシステムはないです。自由に競りをしてください。",
		"相手が実際にそのアイテムや落札金を持っているのか「＠しらべる」で見ることができます。",
	}},
}
