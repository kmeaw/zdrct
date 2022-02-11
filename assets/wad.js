'use strict';

const THING_DATA = [
    [
        "68",
        "Arachnotron"
    ],
    [
        "64",
        "Arch-vile"
    ],
    [
        "3003",
        "Baron of Hell"
    ],
    [
        "3005",
        "Cacodemon"
    ],
    [
        "72",
        "Commander Keen"
    ],
    [
        "16",
        "Cyberdemon"
    ],
    [
        "3002",
        "Demon"
    ],
    [
        "65",
        "Heavy weapon dude"
    ],
    [
        "69",
        "Hell knight"
    ],
    [
        "3001",
        "Imp"
    ],
    [
        "3006",
        "Lost soul"
    ],
    [
        "67",
        "Mancubus"
    ],
    [
        "71",
        "Pain elemental"
    ],
    [
        "66",
        "Revenant"
    ],
    [
        "9",
        "Shotgun guy"
    ],
    [
        "58",
        "Spectre"
    ],
    [
        "7",
        "Spiderdemon"
    ],
    [
        "84",
        "Wolfenstein SS"
    ],
    [
        "3004",
        "Zombieman"
    ],
    [
        "2006",
        "BFG9000"
    ],
    [
        "2002",
        "Chaingun"
    ],
    [
        "2005",
        "Chainsaw"
    ],
    [
        "2004",
        "Plasma gun"
    ],
    [
        "2003",
        "Rocket launcher"
    ],
    [
        "2001",
        "Shotgun"
    ],
    [
        "82",
        "Super shotgun"
    ],
    [
        "2008",
        "4 shotgun shells"
    ],
    [
        "2048",
        "Box of bullets"
    ],
    [
        "2046",
        "Box of rockets"
    ],
    [
        "2049",
        "Box of shotgun shells"
    ],
    [
        "2007",
        "Clip"
    ],
    [
        "2047",
        "Energy cell"
    ],
    [
        "17",
        "Energy cell pack"
    ],
    [
        "2010",
        "Rocket"
    ],
    [
        "2015",
        "Armor bonus"
    ],
    [
        "2023",
        "Berserk"
    ],
    [
        "2026",
        "Computer area map"
    ],
    [
        "2014",
        "Health bonus"
    ],
    [
        "2022",
        "Invulnerability"
    ],
    [
        "2045",
        "Light amplification visor"
    ],
    [
        "83",
        "Megasphere"
    ],
    [
        "2024",
        "Partial invisibility"
    ],
    [
        "2013",
        "Supercharge"
    ],
    [
        "2018",
        "Armor"
    ],
    [
        "8",
        "Backpack"
    ],
    [
        "2012",
        "Medikit"
    ],
    [
        "2019",
        "Megaarmor"
    ],
    [
        "2025",
        "Radiation shielding suit"
    ],
    [
        "2011",
        "Stimpack"
    ],
    [
        "5",
        "Blue keycard"
    ],
    [
        "40",
        "Blue skull key"
    ],
    [
        "13",
        "Red keycard"
    ],
    [
        "38",
        "Red skull key"
    ],
    [
        "6",
        "Yellow keycard"
    ],
    [
        "39",
        "Yellow skull key"
    ],
    [
        "47",
        "Brown stump"
    ],
    [
        "70",
        "Burning barrel"
    ],
    [
        "43",
        "Burnt tree"
    ],
    [
        "35",
        "Candelabra"
    ],
    [
        "41",
        "Evil eye"
    ],
    [
        "2035",
        "Exploding barrel"
    ],
    [
        "28",
        "Five skulls \"shish kebab\""
    ],
    [
        "42",
        "Floating skull"
    ],
    [
        "2028",
        "Floor lamp"
    ],
    [
        "53",
        "Hanging leg"
    ],
    [
        "52",
        "Hanging pair of legs"
    ],
    [
        "78",
        "Hanging torso, brain removed"
    ],
    [
        "75",
        "Hanging torso, looking down"
    ],
    [
        "77",
        "Hanging torso, looking up"
    ],
    [
        "76",
        "Hanging torso, open skull"
    ],
    [
        "50",
        "Hanging victim, arms out"
    ],
    [
        "74",
        "Hanging victim, guts and brain removed"
    ],
    [
        "73",
        "Hanging victim, guts removed"
    ],
    [
        "51",
        "Hanging victim, one-legged"
    ],
    [
        "49",
        "Hanging victim, twitching"
    ],
    [
        "25",
        "Impaled human"
    ],
    [
        "54",
        "Large brown tree"
    ],
    [
        "29",
        "Pile of skulls and candles"
    ],
    [
        "55",
        "Short blue firestick"
    ],
    [
        "56",
        "Short green firestick"
    ],
    [
        "31",
        "Short green pillar"
    ],
    [
        "36",
        "Short green pillar with beating heart"
    ],
    [
        "57",
        "Short red firestick"
    ],
    [
        "33",
        "Short red pillar"
    ],
    [
        "37",
        "Short red pillar with skull"
    ],
    [
        "86",
        "Short techno floor lamp"
    ],
    [
        "27",
        "Skull on a pole"
    ],
    [
        "44",
        "Tall blue firestick"
    ],
    [
        "45",
        "Tall green firestick"
    ],
    [
        "30",
        "Tall green pillar"
    ],
    [
        "46",
        "Tall red firestick"
    ],
    [
        "32",
        "Tall red pillar"
    ],
    [
        "48",
        "Tall techno column"
    ],
    [
        "85",
        "Tall techno floor lamp"
    ],
    [
        "26",
        "Twitching impaled human"
    ],
    [
        "10",
        "Bloody mess"
    ],
    [
        "12",
        "Bloody mess 2"
    ],
    [
        "34",
        "Candle"
    ],
    [
        "22",
        "Dead cacodemon"
    ],
    [
        "21",
        "Dead demon"
    ],
    [
        "18",
        "Dead former human"
    ],
    [
        "19",
        "Dead former sergeant"
    ],
    [
        "20",
        "Dead imp"
    ],
    [
        "23",
        "Dead lost soul (invisible)"
    ],
    [
        "15",
        "Dead player"
    ],
    [
        "62",
        "Hanging leg"
    ],
    [
        "60",
        "Hanging pair of legs"
    ],
    [
        "59",
        "Hanging victim, arms out"
    ],
    [
        "61",
        "Hanging victim, one-legged"
    ],
    [
        "63",
        "Hanging victim, twitching"
    ],
    [
        "79",
        "Pool of blood"
    ],
    [
        "80",
        "Pool of blood"
    ],
    [
        "24",
        "Pool of blood and flesh"
    ],
    [
        "81",
        "Pool of brains"
    ],
    [
        "11",
        "Deathmatch start"
    ],
    [
        "89",
        "Monster spawner"
    ],
    [
        "1",
        "Player 1 start"
    ],
    [
        "2",
        "Player 2 start"
    ],
    [
        "3",
        "Player 3 start"
    ],
    [
        "4",
        "Player 4 start"
    ],
    [
        "88",
        "Romero's head"
    ],
    [
        "87",
        "Spawn spot"
    ],
    [
        "14",
        "Teleport landing"
    ],
    [
        "7",
        "D'Sparil"
    ],
    [
        "15",
        "Disciple of D'Sparil"
    ],
    [
        "5",
        "Fire gargoyle"
    ],
    [
        "66",
        "Gargoyle"
    ],
    [
        "68",
        "Golem"
    ],
    [
        "69",
        "Golem ghost"
    ],
    [
        "6",
        "Iron lich"
    ],
    [
        "9",
        "Maulotaur"
    ],
    [
        "45",
        "Nitrogolem"
    ],
    [
        "46",
        "Nitrogolem ghost"
    ],
    [
        "92",
        "Ophidian"
    ],
    [
        "90",
        "Sabreclaw"
    ],
    [
        "64",
        "Undead Warrior"
    ],
    [
        "65",
        "Undead Warrior ghost"
    ],
    [
        "70",
        "Weredragon"
    ],
    [
        "53",
        "Dragon Claw"
    ],
    [
        "2001",
        "Ethereal Crossbow"
    ],
    [
        "2002",
        "Firemace"
    ],
    [
        "2005",
        "Gauntlets of the Necromancer"
    ],
    [
        "2004",
        "Hellstaff"
    ],
    [
        "2003",
        "Phoenix Rod"
    ],
    [
        "54",
        "Claw Orb"
    ],
    [
        "12",
        "Crystal Geode"
    ],
    [
        "55",
        "Energy Orb"
    ],
    [
        "18",
        "Ethereal Arrows"
    ],
    [
        "22",
        "Flame Orb"
    ],
    [
        "21",
        "Greater Runes"
    ],
    [
        "23",
        "Inferno Orb"
    ],
    [
        "20",
        "Lesser Runes"
    ],
    [
        "13",
        "Mace Spheres"
    ],
    [
        "16",
        "Pile of Mace Spheres"
    ],
    [
        "19",
        "Quiver of Ethereal Arrows"
    ],
    [
        "10",
        "Wand Crystal"
    ],
    [
        "8",
        "Bag of Holding"
    ],
    [
        "36",
        "Chaos Device"
    ],
    [
        "35",
        "Map Scroll"
    ],
    [
        "30",
        "Morph Ovum"
    ],
    [
        "32",
        "Mystic Urn"
    ],
    [
        "82",
        "Quartz Flask"
    ],
    [
        "84",
        "Ring of Invincibility"
    ],
    [
        "75",
        "Shadowsphere"
    ],
    [
        "34",
        "Timebomb of the Ancients"
    ],
    [
        "86",
        "Tome of Power"
    ],
    [
        "33",
        "Torch"
    ],
    [
        "83",
        "Wings of Wrath"
    ],
    [
        "81",
        "Crystal Vial"
    ],
    [
        "31",
        "Enchanted Shield"
    ],
    [
        "85",
        "Silver Shield"
    ],
    [
        "79",
        "Blue key"
    ],
    [
        "73",
        "Green key"
    ],
    [
        "80",
        "Yellow key"
    ],
    [
        "44",
        "Barrel"
    ],
    [
        "76",
        "Fire brazier"
    ],
    [
        "51",
        "Hanging corpse"
    ],
    [
        "40",
        "Large stalactite"
    ],
    [
        "38",
        "Large stalagmite"
    ],
    [
        "27",
        "Serpent torch"
    ],
    [
        "29",
        "Short grey pillar"
    ],
    [
        "39",
        "Small stalactite"
    ],
    [
        "37",
        "Small stalagmite"
    ],
    [
        "47",
        "Tall brown pillar"
    ],
    [
        "87",
        "Volcano"
    ],
    [
        "28",
        "Chandelier"
    ],
    [
        "17",
        "Hanging skull (long rope)"
    ],
    [
        "24",
        "Hanging skull (medium rope)"
    ],
    [
        "25",
        "Hanging skull (short rope)"
    ],
    [
        "26",
        "Hanging skull (shortest rope)"
    ],
    [
        "49",
        "Moss 1 string"
    ],
    [
        "48",
        "Moss 2 strings"
    ],
    [
        "50",
        "Wall torch"
    ],
    [
        "1205",
        "Bells"
    ],
    [
        "1202",
        "Drops"
    ],
    [
        "1209",
        "Fast footsteps"
    ],
    [
        "1206",
        "Growl"
    ],
    [
        "1204",
        "Heart beat"
    ],
    [
        "1208",
        "Laughter"
    ],
    [
        "1207",
        "Magic"
    ],
    [
        "1200",
        "Scream"
    ],
    [
        "1203",
        "Slow footsteps"
    ],
    [
        "1201",
        "Squish"
    ],
    [
        "41",
        "Waterfall"
    ],
    [
        "42",
        "Wind"
    ],
    [
        "56",
        "D'Sparil teleport spot"
    ],
    [
        "11",
        "Deathmatch start"
    ],
    [
        "94",
        "Key gizmo (blue)"
    ],
    [
        "95",
        "Key gizmo (green)"
    ],
    [
        "96",
        "Key gizmo (yellow)"
    ],
    [
        "1",
        "Player 1 start"
    ],
    [
        "2",
        "Player 2 start"
    ],
    [
        "3",
        "Player 3 start"
    ],
    [
        "4",
        "Player 4 start"
    ],
    [
        "2035",
        "Pod"
    ],
    [
        "43",
        "Pod generator"
    ],
    [
        "52",
        "Teleport glitter (blue)"
    ],
    [
        "74",
        "Teleport glitter (red)"
    ],
    [
        "14",
        "Teleport landing"
    ],
    [
        "10060",
        "Afrit"
    ],
    [
        "8080",
        "Brown chaos serpent"
    ],
    [
        "107",
        "Centaur"
    ],
    [
        "114",
        "Dark bishop"
    ],
    [
        "254",
        "Death wyvern"
    ],
    [
        "10030",
        "Ettin"
    ],
    [
        "31",
        "Green chaos serpent"
    ],
    [
        "10080",
        "Heresiarch"
    ],
    [
        "10200",
        "Korax"
    ],
    [
        "10102",
        "Menelkir"
    ],
    [
        "34",
        "Reiver"
    ],
    [
        "10011",
        "Reiver (Buried)"
    ],
    [
        "115",
        "Slaughtaur"
    ],
    [
        "121",
        "Stalker"
    ],
    [
        "120",
        "Stalker boss"
    ],
    [
        "10101",
        "Traductus"
    ],
    [
        "8020",
        "Wendigo"
    ],
    [
        "10100",
        "Zedek"
    ],
    [
        "8040",
        "Arc of Death"
    ],
    [
        "21",
        "Bloodscourge (Skull)"
    ],
    [
        "23",
        "Bloodscourge (Stick)"
    ],
    [
        "22",
        "Bloodscourge (Stub)"
    ],
    [
        "8009",
        "Firestorm"
    ],
    [
        "53",
        "Frost Shards"
    ],
    [
        "123",
        "Hammer of Retribution"
    ],
    [
        "12",
        "Quietus (Blade)"
    ],
    [
        "13",
        "Quietus (Cross)"
    ],
    [
        "16",
        "Quietus (Hilt)"
    ],
    [
        "10",
        "Serpent Staff"
    ],
    [
        "8010",
        "Timon's Axe"
    ],
    [
        "18",
        "Wraithverge (Arc)"
    ],
    [
        "19",
        "Wraithverge (Cross)"
    ],
    [
        "20",
        "Wraithverge (Shaft)"
    ],
    [
        "122",
        "Blue mana"
    ],
    [
        "8004",
        "Combined mana"
    ],
    [
        "124",
        "Green mana"
    ],
    [
        "10040",
        "Banishment Device"
    ],
    [
        "8002",
        "Boots of Speed"
    ],
    [
        "36",
        "Chaos Device"
    ],
    [
        "86",
        "Dark Servant"
    ],
    [
        "10110",
        "Disc of Repulsion"
    ],
    [
        "8041",
        "Dragonskin Bracers"
    ],
    [
        "8000",
        "Fléchette"
    ],
    [
        "84",
        "Icon of the Defender"
    ],
    [
        "8003",
        "Krater of Might"
    ],
    [
        "10120",
        "Mystic Ambit Incant"
    ],
    [
        "32",
        "Mystic Urn"
    ],
    [
        "30",
        "Porkalator"
    ],
    [
        "82",
        "Quartz Flask"
    ],
    [
        "33",
        "Torch"
    ],
    [
        "83",
        "Wings of Wrath"
    ],
    [
        "8008",
        "Amulet of Warding"
    ],
    [
        "81",
        "Crystal Vial"
    ],
    [
        "8006",
        "Falcon Shield"
    ],
    [
        "8005",
        "Mesh Armor"
    ],
    [
        "8007",
        "Platinum Helm"
    ],
    [
        "8032",
        "Axe Key"
    ],
    [
        "8200",
        "Castle Key"
    ],
    [
        "8031",
        "Cave Key"
    ],
    [
        "9021",
        "Clock Gear (Bronze in Steel)"
    ],
    [
        "9019",
        "Clock Gear (Bronze)"
    ],
    [
        "9020",
        "Clock Gear (Steel in Bronze)"
    ],
    [
        "9018",
        "Clock Gear (Steel)"
    ],
    [
        "9007",
        "Daemon Codex"
    ],
    [
        "8035",
        "Dungeon Key"
    ],
    [
        "8034",
        "Emerald Key"
    ],
    [
        "9005",
        "Emerald Planet 1"
    ],
    [
        "9009",
        "Emerald Planet 2"
    ],
    [
        "8033",
        "Fire Key"
    ],
    [
        "9014",
        "Flame Mask"
    ],
    [
        "9015",
        "Glaive Seal"
    ],
    [
        "9003",
        "Heart of D'Sparil"
    ],
    [
        "9016",
        "Holy Relic"
    ],
    [
        "8038",
        "Horn Key"
    ],
    [
        "9008",
        "Liber Oscura"
    ],
    [
        "9004",
        "Ruby Planet"
    ],
    [
        "8037",
        "Rusty Key"
    ],
    [
        "9006",
        "Sapphire Planet 1"
    ],
    [
        "9010",
        "Sapphire Planet 2"
    ],
    [
        "9017",
        "Sigil of the Magus"
    ],
    [
        "8036",
        "Silver Key"
    ],
    [
        "8030",
        "Steel Key"
    ],
    [
        "8039",
        "Swamp Key"
    ],
    [
        "9002",
        "Yorick's Skull"
    ],
    [
        "8100",
        "Barrel"
    ],
    [
        "77",
        "Battle rag banner"
    ],
    [
        "8065",
        "Bell"
    ],
    [
        "99",
        "Black rock"
    ],
    [
        "8061",
        "Brazier with flame"
    ],
    [
        "8051",
        "Bronze gargoyle (short)"
    ],
    [
        "8047",
        "Bronze gargoyle (tall)"
    ],
    [
        "97",
        "Brown rock (large)"
    ],
    [
        "98",
        "Brown rock (small)"
    ],
    [
        "8069",
        "Cauldron (lit)"
    ],
    [
        "8070",
        "Cauldron (unlit)"
    ],
    [
        "8049",
        "Dark lava gargoyle (short)"
    ],
    [
        "8045",
        "Dark lava gargoyle (tall)"
    ],
    [
        "24",
        "Dead tree"
    ],
    [
        "8062",
        "Destructible tree"
    ],
    [
        "8068",
        "Evergreen tree"
    ],
    [
        "80",
        "Gnarled tree 1"
    ],
    [
        "87",
        "Gnarled tree 2"
    ],
    [
        "8103",
        "Hanging bucket"
    ],
    [
        "71",
        "Hanging corpse"
    ],
    [
        "76",
        "Ice gargoyle (short)"
    ],
    [
        "73",
        "Ice gargoyle (tall)"
    ],
    [
        "93",
        "Ice spike (large)"
    ],
    [
        "94",
        "Ice spike (medium)"
    ],
    [
        "95",
        "Ice spike (small)"
    ],
    [
        "96",
        "Ice spike (tiny)"
    ],
    [
        "89",
        "Icicle (large)"
    ],
    [
        "90",
        "Icicle (medium)"
    ],
    [
        "91",
        "Icicle (small)"
    ],
    [
        "92",
        "Icicle (tiny)"
    ],
    [
        "61",
        "Impaled corpse"
    ],
    [
        "8067",
        "Iron maiden"
    ],
    [
        "25",
        "Leafless tree"
    ],
    [
        "8050",
        "Light lava gargoyle (short)"
    ],
    [
        "8046",
        "Light lava gargoyle (tall)"
    ],
    [
        "108",
        "Lynched corpse"
    ],
    [
        "109",
        "Lynched corpse (heartless)"
    ],
    [
        "8042",
        "Minotaur statue (lit)"
    ],
    [
        "8043",
        "Minotaur statue (unlit)"
    ],
    [
        "60",
        "Mossy dead tree"
    ],
    [
        "15",
        "Mossy rock (large)"
    ],
    [
        "26",
        "Mossy tree 1"
    ],
    [
        "27",
        "Mossy tree 2"
    ],
    [
        "9012",
        "Pedestal of D'Sparil"
    ],
    [
        "103",
        "Pillar with vase"
    ],
    [
        "105",
        "Pot (medium)"
    ],
    [
        "106",
        "Pot (short)"
    ],
    [
        "104",
        "Pot (tall)"
    ],
    [
        "8044",
        "Rusty gargoyle (tall)"
    ],
    [
        "8102",
        "Shrub (large)"
    ],
    [
        "8101",
        "Shrub (small)"
    ],
    [
        "110",
        "Sitting corpse"
    ],
    [
        "8060",
        "Skull with flame"
    ],
    [
        "52",
        "Stalactite (large)"
    ],
    [
        "56",
        "Stalactite (medium)"
    ],
    [
        "57",
        "Stalactite (small)"
    ],
    [
        "49",
        "Stalagmite (large)"
    ],
    [
        "50",
        "Stalagmite (medium)"
    ],
    [
        "51",
        "Stalagmite (small)"
    ],
    [
        "48",
        "Stalagmite pillar"
    ],
    [
        "8052",
        "Steel gargoyle (short)"
    ],
    [
        "8048",
        "Steel gargoyle (tall)"
    ],
    [
        "74",
        "Stone gargoyle (short)"
    ],
    [
        "72",
        "Stone gargoyle (tall)"
    ],
    [
        "8064",
        "Suit of armor"
    ],
    [
        "78",
        "Tall tree 1"
    ],
    [
        "79",
        "Tall tree 2"
    ],
    [
        "69",
        "Tombstone (Brian P)"
    ],
    [
        "66",
        "Tombstone (Brian R)"
    ],
    [
        "63",
        "Tombstone (RIP)"
    ],
    [
        "64",
        "Tombstone (Shane)"
    ],
    [
        "67",
        "Tombstone (cross circle)"
    ],
    [
        "65",
        "Tombstone (slimy)"
    ],
    [
        "68",
        "Tombstone (small cross)"
    ],
    [
        "88",
        "Tree log"
    ],
    [
        "29",
        "Tree stump (bare)"
    ],
    [
        "28",
        "Tree stump (burned)"
    ],
    [
        "116",
        "Twined torch (lit)"
    ],
    [
        "117",
        "Twined torch (unlit)"
    ],
    [
        "5",
        "Winged statue"
    ],
    [
        "9011",
        "Yorick's statue"
    ],
    [
        "119",
        "3 Candles (lit)"
    ],
    [
        "8066",
        "Blue candle (lit)"
    ],
    [
        "100",
        "Brick rubble (large)"
    ],
    [
        "102",
        "Brick rubble (medium)"
    ],
    [
        "101",
        "Brick rubble (small)"
    ],
    [
        "8504",
        "Candle w/o web (unlit)"
    ],
    [
        "8502",
        "Candle with web (unlit)"
    ],
    [
        "8072",
        "Chain (long)"
    ],
    [
        "8071",
        "Chain (short)"
    ],
    [
        "8074",
        "Chain with large hook"
    ],
    [
        "8075",
        "Chain with small hook"
    ],
    [
        "8076",
        "Chain with spike ball"
    ],
    [
        "17",
        "Chandelier (lit)"
    ],
    [
        "8063",
        "Chandelier (unlit)"
    ],
    [
        "8507",
        "Goblet (short)"
    ],
    [
        "8508",
        "Goblet (silver)"
    ],
    [
        "8505",
        "Goblet (spilled)"
    ],
    [
        "8506",
        "Goblet (tall)"
    ],
    [
        "8503",
        "Gray candle (unlit)"
    ],
    [
        "58",
        "Hanging moss 1"
    ],
    [
        "59",
        "Hanging moss 2"
    ],
    [
        "8073",
        "Hook with heart"
    ],
    [
        "8077",
        "Hook with skull"
    ],
    [
        "10503",
        "Large flame"
    ],
    [
        "10502",
        "Large flame (timed)"
    ],
    [
        "39",
        "Large mushroom 1"
    ],
    [
        "40",
        "Large mushroom 2"
    ],
    [
        "8509",
        "Meat cleaver"
    ],
    [
        "41",
        "Medium mushroom"
    ],
    [
        "8501",
        "Metal beer stein"
    ],
    [
        "9",
        "Mossy rock (medium)"
    ],
    [
        "7",
        "Mossy rock (small)"
    ],
    [
        "6",
        "Mossy rock (tiny)"
    ],
    [
        "111",
        "Pool of blood"
    ],
    [
        "62",
        "Sleeping corpse"
    ],
    [
        "10501",
        "Small flame"
    ],
    [
        "10500",
        "Small flame (timed)"
    ],
    [
        "42",
        "Small mushroom 1"
    ],
    [
        "44",
        "Small mushroom 2"
    ],
    [
        "45",
        "Small mushroom 3"
    ],
    [
        "46",
        "Small mushroom 4"
    ],
    [
        "47",
        "Small mushroom 5"
    ],
    [
        "37",
        "Tree stump 1"
    ],
    [
        "38",
        "Tree stump 2"
    ],
    [
        "54",
        "Wall torch (lit)"
    ],
    [
        "55",
        "Wall torch (unlit)"
    ],
    [
        "8500",
        "Wooden beer stein"
    ],
    [
        "1403",
        "Creak"
    ],
    [
        "1408",
        "Earth"
    ],
    [
        "1401",
        "Heavy"
    ],
    [
        "1407",
        "Ice"
    ],
    [
        "1405",
        "Lava"
    ],
    [
        "1402",
        "Metal"
    ],
    [
        "1409",
        "Metal2"
    ],
    [
        "1404",
        "Silence"
    ],
    [
        "1400",
        "Stone"
    ],
    [
        "1406",
        "Water"
    ],
    [
        "1410",
        "Wind"
    ],
    [
        "10225",
        "Bat spawner"
    ],
    [
        "11",
        "Deathmatch start"
    ],
    [
        "10003",
        "Fog patch (large)"
    ],
    [
        "10002",
        "Fog patch (medium)"
    ],
    [
        "10001",
        "Fog patch (small)"
    ],
    [
        "10000",
        "Fog spawner"
    ],
    [
        "118",
        "Glitter bridge"
    ],
    [
        "113",
        "Leaf spawner"
    ],
    [
        "9001",
        "Map spot"
    ],
    [
        "9013",
        "Map spot (gravity)"
    ],
    [
        "1",
        "Player 1 start"
    ],
    [
        "2",
        "Player 2 start"
    ],
    [
        "3",
        "Player 3 start"
    ],
    [
        "4",
        "Player 4 start"
    ],
    [
        "9100",
        "Player 5 start"
    ],
    [
        "9101",
        "Player 6 start"
    ],
    [
        "9102",
        "Player 7 start"
    ],
    [
        "9103",
        "Player 8 start"
    ],
    [
        "8104",
        "Poisonous mushroom"
    ],
    [
        "3000",
        "Polyobject anchor"
    ],
    [
        "3001",
        "Polyobject start spot"
    ],
    [
        "3002",
        "Polyobject start spot (crush)"
    ],
    [
        "10090",
        "Spike Down"
    ],
    [
        "10091",
        "Spike Up"
    ],
    [
        "14",
        "Teleport landing"
    ],
    [
        "140",
        "Teleport smoke"
    ],
    [
        "231",
        "Acolyte (blue)"
    ],
    [
        "147",
        "Acolyte (dark green)"
    ],
    [
        "148",
        "Acolyte (gold)"
    ],
    [
        "146",
        "Acolyte (gray)"
    ],
    [
        "232",
        "Acolyte (light green)"
    ],
    [
        "142",
        "Acolyte (red)"
    ],
    [
        "143",
        "Acolyte (rust)"
    ],
    [
        "3002",
        "Acolyte (tan)"
    ],
    [
        "201",
        "Becoming Acolyte"
    ],
    [
        "187",
        "Bishop"
    ],
    [
        "27",
        "Ceiling turret"
    ],
    [
        "3005",
        "Crusader"
    ],
    [
        "128",
        "Entity"
    ],
    [
        "26",
        "Entity nest"
    ],
    [
        "198",
        "Entity pod"
    ],
    [
        "16",
        "Inquisitor"
    ],
    [
        "12",
        "Loremaster"
    ],
    [
        "64",
        "Macil"
    ],
    [
        "200",
        "Macil Spectre"
    ],
    [
        "199",
        "Oracle"
    ],
    [
        "71",
        "Programmer"
    ],
    [
        "3001",
        "Reaver"
    ],
    [
        "3006",
        "Sentinel"
    ],
    [
        "58",
        "Shadow Acolyte"
    ],
    [
        "75",
        "Spectre (Bishop)"
    ],
    [
        "168",
        "Spectre (Loremaster)"
    ],
    [
        "167",
        "Spectre (Macil)"
    ],
    [
        "76",
        "Spectre (Oracle)"
    ],
    [
        "129",
        "Spectre (Programmer)"
    ],
    [
        "186",
        "Stalker"
    ],
    [
        "3003",
        "Templar"
    ],
    [
        "73",
        "Armorer"
    ],
    [
        "72",
        "Barkeep"
    ],
    [
        "141",
        "Beggar 1"
    ],
    [
        "155",
        "Beggar 2"
    ],
    [
        "156",
        "Beggar 3"
    ],
    [
        "157",
        "Beggar 4"
    ],
    [
        "158",
        "Beggar 5"
    ],
    [
        "204",
        "Kneeling Guy"
    ],
    [
        "74",
        "Medic"
    ],
    [
        "181",
        "Peasant Blue"
    ],
    [
        "172",
        "Peasant Dark Green 1"
    ],
    [
        "173",
        "Peasant Dark Green 2"
    ],
    [
        "174",
        "Peasant Dark Green 3"
    ],
    [
        "178",
        "Peasant Gold 1"
    ],
    [
        "179",
        "Peasant Gold 2"
    ],
    [
        "180",
        "Peasant Gold 3"
    ],
    [
        "66",
        "Peasant Gray 1"
    ],
    [
        "134",
        "Peasant Gray 2"
    ],
    [
        "135",
        "Peasant Gray 3"
    ],
    [
        "175",
        "Peasant Light Green 1"
    ],
    [
        "176",
        "Peasant Light Green 2"
    ],
    [
        "177",
        "Peasant Light Green 3"
    ],
    [
        "65",
        "Peasant Red 1"
    ],
    [
        "132",
        "Peasant Red 2"
    ],
    [
        "133",
        "Peasant Red 3"
    ],
    [
        "67",
        "Peasant Rust 1"
    ],
    [
        "136",
        "Peasant Rust 2"
    ],
    [
        "137",
        "Peasant Rust 3"
    ],
    [
        "3004",
        "Peasant Tan 1"
    ],
    [
        "130",
        "Peasant Tan 2"
    ],
    [
        "131",
        "Peasant Tan 3"
    ],
    [
        "9",
        "Rebel 1"
    ],
    [
        "144",
        "Rebel 2"
    ],
    [
        "145",
        "Rebel 3"
    ],
    [
        "149",
        "Rebel 4"
    ],
    [
        "150",
        "Rebel 5"
    ],
    [
        "151",
        "Rebel 6"
    ],
    [
        "116",
        "Weapon smith"
    ],
    [
        "169",
        "Zombie"
    ],
    [
        "170",
        "Zombie spawner"
    ],
    [
        "2002",
        "Assault rifle (lying)"
    ],
    [
        "2006",
        "Assault rifle (standing)"
    ],
    [
        "2001",
        "Crossbow"
    ],
    [
        "2005",
        "Flamethrower"
    ],
    [
        "154",
        "Grenade launcher"
    ],
    [
        "2004",
        "Mauler"
    ],
    [
        "2003",
        "Mini-missile launcher"
    ],
    [
        "77",
        "Sigil 1 (Lightning)"
    ],
    [
        "78",
        "Sigil 2 (Rail)"
    ],
    [
        "79",
        "Sigil 3 (Spread)"
    ],
    [
        "80",
        "Sigil 4 (Column)"
    ],
    [
        "81",
        "Sigil 5 (Blast)"
    ],
    [
        "2048",
        "Box of bullets"
    ],
    [
        "2007",
        "Bullet clip"
    ],
    [
        "2046",
        "Crate of missiles"
    ],
    [
        "114",
        "Electric bolt"
    ],
    [
        "17",
        "Energy pack"
    ],
    [
        "2047",
        "Energy pod"
    ],
    [
        "152",
        "HE grenade"
    ],
    [
        "2010",
        "Mini-missile"
    ],
    [
        "153",
        "Phosphorous grenade"
    ],
    [
        "115",
        "Poison bolt"
    ],
    [
        "138",
        "10 Gold"
    ],
    [
        "139",
        "25 Gold"
    ],
    [
        "140",
        "50 Gold"
    ],
    [
        "183",
        "Ammo satchel"
    ],
    [
        "206",
        "Communicator"
    ],
    [
        "2025",
        "Environmental suit"
    ],
    [
        "93",
        "Gold coin"
    ],
    [
        "2026",
        "Map"
    ],
    [
        "2027",
        "Scanner"
    ],
    [
        "2024",
        "Shadow armor"
    ],
    [
        "207",
        "Targeter"
    ],
    [
        "10",
        "Teleporter beacon"
    ],
    [
        "2018",
        "Leather armor"
    ],
    [
        "2011",
        "Med patch"
    ],
    [
        "2012",
        "Medical kit"
    ],
    [
        "2019",
        "Metal armor"
    ],
    [
        "83",
        "Surgery kit"
    ],
    [
        "230",
        "Base Key"
    ],
    [
        "193",
        "Blue Crystal Key"
    ],
    [
        "39",
        "Brass Key"
    ],
    [
        "226",
        "Broken power coupling"
    ],
    [
        "195",
        "Chapel Key"
    ],
    [
        "182",
        "Computer"
    ],
    [
        "236",
        "Core Key"
    ],
    [
        "59",
        "Degnin ore"
    ],
    [
        "234",
        "Factory Key"
    ],
    [
        "25",
        "Force field guard"
    ],
    [
        "45",
        "Gate piston"
    ],
    [
        "40",
        "Gold Key"
    ],
    [
        "90",
        "Guard uniform"
    ],
    [
        "184",
        "ID Badge"
    ],
    [
        "13",
        "ID Card"
    ],
    [
        "233",
        "Mauler Key"
    ],
    [
        "235",
        "Mine Key"
    ],
    [
        "205",
        "Offering chalice"
    ],
    [
        "52",
        "Officer uniform"
    ],
    [
        "61",
        "Oracle Key"
    ],
    [
        "86",
        "Order Key"
    ],
    [
        "185",
        "Pass Card"
    ],
    [
        "220",
        "Power coupling"
    ],
    [
        "92",
        "Power crystal"
    ],
    [
        "192",
        "Red Crystal Key"
    ],
    [
        "91",
        "Severed Hand"
    ],
    [
        "38",
        "Silver Key"
    ],
    [
        "166",
        "Warehouse Key"
    ],
    [
        "224",
        "Alien asp climber"
    ],
    [
        "221",
        "Alien bubble column"
    ],
    [
        "223",
        "Alien ceiling bubble"
    ],
    [
        "222",
        "Alien floor bubble"
    ],
    [
        "225",
        "Alien spider light"
    ],
    [
        "228",
        "Ammo filler"
    ],
    [
        "194",
        "Anvil"
    ],
    [
        "54",
        "Aztec pillar"
    ],
    [
        "69",
        "Barricade column"
    ],
    [
        "202",
        "Big tree"
    ],
    [
        "197",
        "Brass tech lamp"
    ],
    [
        "70",
        "Burning barrel"
    ],
    [
        "105",
        "Burning bowl"
    ],
    [
        "106",
        "Burning brazier"
    ],
    [
        "35",
        "Candelabra"
    ],
    [
        "162",
        "Cave pillar bottom"
    ],
    [
        "159",
        "Cave pillar top"
    ],
    [
        "63",
        "Chimneystack"
    ],
    [
        "55",
        "Damaged aztec pillar"
    ],
    [
        "94",
        "Exploding barrel"
    ],
    [
        "2028",
        "Globe light"
    ],
    [
        "113",
        "Hearts in tank"
    ],
    [
        "227",
        "Huge alien pillar"
    ],
    [
        "209",
        "Huge tank 1 with skeleton"
    ],
    [
        "210",
        "Huge tank 2"
    ],
    [
        "211",
        "Huge tank 3"
    ],
    [
        "57",
        "Huge tech pillar"
    ],
    [
        "50",
        "Huge torch"
    ],
    [
        "47",
        "Large torch"
    ],
    [
        "111",
        "Medium torch"
    ],
    [
        "43",
        "Outside lamp"
    ],
    [
        "51",
        "Palm tree"
    ],
    [
        "188",
        "Pitcher"
    ],
    [
        "46",
        "Pole lantern"
    ],
    [
        "165",
        "Pot"
    ],
    [
        "203",
        "Potted tree"
    ],
    [
        "56",
        "Ruined aztec pillar"
    ],
    [
        "44",
        "Ruined statue"
    ],
    [
        "60",
        "Short bush"
    ],
    [
        "196",
        "Silver tech lamp"
    ],
    [
        "98",
        "Stalactite (large)"
    ],
    [
        "161",
        "Stalactite (small)"
    ],
    [
        "160",
        "Stalagmite (large)"
    ],
    [
        "163",
        "Stalagmite (small)"
    ],
    [
        "110",
        "Statue"
    ],
    [
        "189",
        "Stool"
    ],
    [
        "117",
        "Surgery crab"
    ],
    [
        "62",
        "Tall bush"
    ],
    [
        "213",
        "Tank 4 spine with organs"
    ],
    [
        "214",
        "Tank 5 stumpy the acolyte"
    ],
    [
        "229",
        "Tank 6 spectre"
    ],
    [
        "48",
        "Tech pillar"
    ],
    [
        "68",
        "Tray"
    ],
    [
        "33",
        "Tree stub"
    ],
    [
        "82",
        "Wooden barrel"
    ],
    [
        "96",
        "Brass fluorescent light"
    ],
    [
        "28",
        "Cage light"
    ],
    [
        "34",
        "Candle"
    ],
    [
        "109",
        "Ceiling chain"
    ],
    [
        "53",
        "Ceiling water drip"
    ],
    [
        "21",
        "Dead acolyte (disappears)"
    ],
    [
        "22",
        "Dead crusader"
    ],
    [
        "18",
        "Dead peasant (disappears)"
    ],
    [
        "15",
        "Dead player (disappears)"
    ],
    [
        "20",
        "Dead reaver"
    ],
    [
        "19",
        "Dead rebel"
    ],
    [
        "103",
        "Floor water drip"
    ],
    [
        "97",
        "Gold fluorescent light"
    ],
    [
        "24",
        "Klaxon warning light"
    ],
    [
        "190",
        "Metal pot"
    ],
    [
        "164",
        "Mug"
    ],
    [
        "217",
        "Rebel boots"
    ],
    [
        "218",
        "Rebel helmet"
    ],
    [
        "219",
        "Rebel shirt"
    ],
    [
        "99",
        "Rock 1"
    ],
    [
        "100",
        "Rock 2"
    ],
    [
        "101",
        "Rock 3"
    ],
    [
        "102",
        "Rock 4"
    ],
    [
        "29",
        "Rubble 1"
    ],
    [
        "30",
        "Rubble 2"
    ],
    [
        "31",
        "Rubble 3"
    ],
    [
        "32",
        "Rubble 4"
    ],
    [
        "36",
        "Rubble 5"
    ],
    [
        "37",
        "Rubble 6"
    ],
    [
        "41",
        "Rubble 7"
    ],
    [
        "42",
        "Rubble 8"
    ],
    [
        "212",
        "Sacrificed guy"
    ],
    [
        "216",
        "Sigil banner"
    ],
    [
        "95",
        "Silver fluorescent light"
    ],
    [
        "107",
        "Small torch (lit)"
    ],
    [
        "108",
        "Small torch (unlit)"
    ],
    [
        "215",
        "Stick in water"
    ],
    [
        "191",
        "Tub"
    ],
    [
        "2014",
        "Water bottle"
    ],
    [
        "112",
        "Water fountain"
    ],
    [
        "104",
        "Waterfall splash"
    ],
    [
        "11",
        "Deathmatch start"
    ],
    [
        "1",
        "Player 1 start"
    ],
    [
        "2",
        "Player 2 start"
    ],
    [
        "3",
        "Player 3 start"
    ],
    [
        "4",
        "Player 4 start"
    ],
    [
        "208",
        "Practice target"
    ],
    [
        "85",
        "Rat buddy"
    ],
    [
        "14",
        "Teleport landing"
    ],
    [
        "23",
        "Teleport swirl"
    ],
    [
        "7970",
        "Blue chalice"
    ],
    [
        "5130",
        "Blue flag spot"
    ],
    [
        "7968",
        "Blue talisman"
    ],
    [
        "7967",
        "Green talisman"
    ],
    [
        "7975",
        "Ore spawner"
    ],
    [
        "5131",
        "Red flag spot"
    ],
    [
        "7966",
        "Red talisman"
    ],
    [
        "5080",
        "Team Blue start"
    ],
    [
        "5081",
        "Team Red start"
    ],
];

class Sound {
	constructor(iwad, name) {
		this.name = name;
		this.iwad = iwad;
		this.arrayBuffer = iwad.read_lump(name);

		[this.format, this.rate] = new Uint16Array(this.arrayBuffer.slice(0, 4));
		[this.nsamples] = new Uint32Array(this.arrayBuffer.slice(4, 8));
		this.samples = new Uint8Array(this.arrayBuffer.slice(0x18, 0x18 + this.nsamples));
	}

	toWav() {
		const ascii = new TextEncoder('ascii');
		const wav = new Uint8Array(44 + this.samples.length);
		const dataview = new DataView(wav.buffer);
		wav.set(ascii.encode('RIFF'), 0);
		dataview.setUint32(4, 36 + this.samples.length, true);
		wav.set(ascii.encode('WAVEfmt '), 8);
		dataview.setUint32(16, 16, true);
		dataview.setUint16(20, 1, true);
		dataview.setUint16(22, 1, true);
		dataview.setUint32(24, this.rate, true);
		dataview.setUint32(28, this.rate, true);
		dataview.setUint16(32, 1, true);
		dataview.setUint16(34, 8, true);
		wav.set(ascii.encode('data'), 36);
		dataview.setUint32(40, this.samples.length * 2, true);
		wav.set(this.samples, 44);

		return new Blob([wav], {type: 'audio/wav'});
	}

	render($audio) {
		$audio.src = URL.createObjectURL(this.toWav());
	}
}

class Patch {
	constructor(iwad, name) {
		this.name = name;
		this.iwad = iwad;
		this.arrayBuffer = iwad.read_lump(name);
		[this.width, this.height, this.leftoffset, this.topoffset] = new Uint16Array(this.arrayBuffer.slice(0, 8));
		this.columns = new Uint32Array(this.arrayBuffer.slice(8, 8 + 4 * this.width));
		const data = new Uint8Array(this.width * this.height * 4);
		this.columns.forEach((offset, ic) => {
			const column = this.arrayBuffer.slice(offset);
			for (let i = 0; i + 4 < column.byteLength;) {
				const [topdelta, length] = new Uint8Array(column.slice(i, i + 2));
				if (topdelta == 0xFF) break;
				i += 3; // skip unused byte
				const pixels = new Uint8Array(column.slice(i, i + length));
				pixels.forEach((v, j) => {
					j += topdelta;
					data[(ic + j * this.width) * 4 + 0] = iwad.playpal[v * 3 + 0];
					data[(ic + j * this.width) * 4 + 1] = iwad.playpal[v * 3 + 1];
					data[(ic + j * this.width) * 4 + 2] = iwad.playpal[v * 3 + 2];
					data[(ic + j * this.width) * 4 + 3] = 255;
				});
				i += length + 1;
			}
		});
		this.imageData = new ImageData(new Uint8ClampedArray(data.buffer), this.width, this.height);
	}

	render($canvas, size) {
		let dx = 0, dy = 0, dw = this.width, dh = this.height;
		if (size) {
			$canvas.width = size;
			$canvas.height = size;
			const r = size / Math.min(dw, dh);
			dw *= r;
			dh *= r;
			dx = size / 2 - dw / 2;
			dy = size / 2 - dh / 2;
		} else {
			$canvas.width = this.width;
			$canvas.height = this.height;
		}
		const context = $canvas.getContext('2d');
		createImageBitmap(this.imageData).then((renderer) => {
			context.drawImage(renderer, dx, dy, dw, dh);
		});
	}
}

class IWAD {
	constructor(arrayBuffer) {
		this.arrayBuffer = arrayBuffer;
		this.files = {};
		this.patches = {};
		this.sounds = {};

		const ascii = new TextDecoder('ascii');
		const sig4 = ascii.decode(new Uint8Array(arrayBuffer, 0, 4));
		const [lumps, dir_offset] = new Uint32Array(arrayBuffer, 4, 8);
		if (sig4 != 'IWAD') {
			throw 'Bad input file header: ' + sig4;
		}

		if (dir_offset + 16 > arrayBuffer.byteLength) {
			throw 'Bad info table offset: ' + dir_offset;
		}

		let is_patch = false;
		let idx = 0;
		const patchlist = [];

		for (let offset = dir_offset; offset + 16 < arrayBuffer.byteLength; offset += 16) {
			const [pos, size] = new Uint32Array(arrayBuffer, offset, 8);
			let name = ascii.decode(new Uint8Array(arrayBuffer, offset + 8, 8));
			const namezidx = name.indexOf('\0');
			if (namezidx != -1) {
				name = name.substr(0, namezidx);
			}
			idx++;

			if (/^S\d?_START$/.exec(name)) {
				is_patch = true;
				continue;
			} else if (/^S\d?_END$/.exec(name)) {
				is_patch = false;
				continue;
			}
			this.files[name] = {
				pos: pos,
				size: size,
				index: idx,
			};
			if (is_patch) {
				patchlist.push(name);
			} else if (size > 8) {
				if (ascii.decode(new Uint8Array(arrayBuffer, pos, 2)) == 'DS') {
					this.sounds[name] = new Sound(this, name);
				} else {
					const [fmt, rate] = new Uint16Array(arrayBuffer, pos, 2);
					if (fmt == 3 && rate > 0 && (rate % 11025) == 0) {
						this.sounds[name] = new Sound(this, name);
					}
				}
			}
		}
		this.playpal = new Uint8Array(this.read_lump('PLAYPAL'));
		patchlist.forEach((name) => {
			this.patches[name] = new Patch(this, name);
		});
	}

	read_lump(name) {
		const file = this.files[name];
		if (!file) {
			throw 'File not found: ' + name;
		}
		if (file.pos + file.size > this.arrayBuffer.byteLength) {
			throw 'File ' + name + ' has invalid offset or position';
		}
		return this.arrayBuffer.slice(file.pos, file.pos + file.size);
	}
}
