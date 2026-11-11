// Package factions manages player factions: goofy generated names and emoji
// glyphs. Nothing here is safe for concurrent use; the server room serializes
// access.
package factions

import "math/rand"

var adjectives = []string{
	"Damp", "Salty", "Grumpy", "Sleepy", "Soggy", "Wobbly", "Crunchy", "Fluffy",
	"Sneaky", "Dizzy", "Bouncy", "Cranky", "Squishy", "Spicy", "Lumpy", "Jolly",
	"Frosty", "Gassy", "Quirky", "Rowdy", "Sticky", "Tipsy", "Zesty", "Moody",
	"Chunky", "Giggly", "Itchy", "Mushy", "Nervous", "Peppy", "Rusty", "Shiny",
	"Slippery", "Smelly", "Snappy", "Stubborn", "Twitchy", "Wiggly", "Fancy", "Lazy",
	"Brave", "Clumsy", "Dusty", "Feisty",
}

var nouns = []string{
	"Pigeons", "Tuesdays", "Waffles", "Llamas", "Pickles", "Walruses", "Noodles", "Badgers",
	"Biscuits", "Goblins", "Muffins", "Penguins", "Turnips", "Otters", "Potatoes", "Gophers",
	"Pancakes", "Ferrets", "Sandwiches", "Moose", "Sprouts", "Hamsters", "Pretzels", "Toasters",
	"Narwhals", "Cabbages", "Raccoons", "Donuts", "Wombats", "Meatballs", "Sloths", "Crumpets",
	"Gremlins", "Mondays", "Parsnips", "Platypuses", "Dumplings", "Hedgehogs", "Burritos", "Yetis",
	"Lobsters", "Marshmallows",
}

// GenerateName returns an "Adjective Plural-Noun" name. Deterministic for a
// seeded rng.
func GenerateName(rng *rand.Rand) string {
	return adjectives[rng.Intn(len(adjectives))] + " " + nouns[rng.Intn(len(nouns))]
}

// glyphPool is the set of emoji faction glyphs (no skin-tone or ZWJ sequences).
var glyphPool = []string{
	"🦊", "🐙", "🦄", "🐸", "🐼", "🦉", "🐢", "🦀", "🐝", "🦋",
	"🐧", "🦁", "🐯", "🐨", "🐵", "🐔", "🦆", "🦅", "🐺", "🐗",
	"🐴", "🦓", "🦒", "🐘", "🦏", "🐪", "🐿", "🦔", "🐳", "🐬",
	"🦈", "🐊", "🦖", "🐌", "🐞", "🦂", "🍄", "🌵", "🌻", "🍀",
	"🍕", "🍩", "🍉", "🍓", "🌮", "🚀", "⚡", "🔥", "🌈", "💎",
}
