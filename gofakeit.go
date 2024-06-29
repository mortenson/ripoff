package ripoff

import (
	"fmt"

	"github.com/brianvoe/gofakeit/v7"
)

// Calls a `func() string` shaped method in a given faker instance.
func callFakerMethod(method string, faker *gofakeit.Faker) (string, error) {
	switch method {
	case "achAccount":
		return faker.AchAccount(), nil
	case "achRouting":
		return faker.AchRouting(), nil
	case "adjective":
		return faker.Adjective(), nil
	case "adjectiveDemonstrative":
		return faker.AdjectiveDemonstrative(), nil
	case "adjectiveDescriptive":
		return faker.AdjectiveDescriptive(), nil
	case "adjectiveIndefinite":
		return faker.AdjectiveIndefinite(), nil
	case "adjectiveInterrogative":
		return faker.AdjectiveInterrogative(), nil
	case "adjectivePossessive":
		return faker.AdjectivePossessive(), nil
	case "adjectiveProper":
		return faker.AdjectiveProper(), nil
	case "adjectiveQuantitative":
		return faker.AdjectiveQuantitative(), nil
	case "adverb":
		return faker.Adverb(), nil
	case "adverbDegree":
		return faker.AdverbDegree(), nil
	case "adverbFrequencyDefinite":
		return faker.AdverbFrequencyDefinite(), nil
	case "adverbFrequencyIndefinite":
		return faker.AdverbFrequencyIndefinite(), nil
	case "adverbManner":
		return faker.AdverbManner(), nil
	case "adverbPlace":
		return faker.AdverbPlace(), nil
	case "adverbTimeDefinite":
		return faker.AdverbTimeDefinite(), nil
	case "adverbTimeIndefinite":
		return faker.AdverbTimeIndefinite(), nil
	case "animal":
		return faker.Animal(), nil
	case "animalType":
		return faker.AnimalType(), nil
	case "appAuthor":
		return faker.AppAuthor(), nil
	case "appName":
		return faker.AppName(), nil
	case "appVersion":
		return faker.AppVersion(), nil
	case "bS":
		return faker.BS(), nil
	case "beerAlcohol":
		return faker.BeerAlcohol(), nil
	case "beerBlg":
		return faker.BeerBlg(), nil
	case "beerHop":
		return faker.BeerHop(), nil
	case "beerIbu":
		return faker.BeerIbu(), nil
	case "beerMalt":
		return faker.BeerMalt(), nil
	case "beerName":
		return faker.BeerName(), nil
	case "beerStyle":
		return faker.BeerStyle(), nil
	case "beerYeast":
		return faker.BeerYeast(), nil
	case "bird":
		return faker.Bird(), nil
	case "bitcoinAddress":
		return faker.BitcoinAddress(), nil
	case "bitcoinPrivateKey":
		return faker.BitcoinPrivateKey(), nil
	case "blurb":
		return faker.Blurb(), nil
	case "bookAuthor":
		return faker.BookAuthor(), nil
	case "bookGenre":
		return faker.BookGenre(), nil
	case "bookTitle":
		return faker.BookTitle(), nil
	case "breakfast":
		return faker.Breakfast(), nil
	case "buzzWord":
		return faker.BuzzWord(), nil
	case "carFuelType":
		return faker.CarFuelType(), nil
	case "carMaker":
		return faker.CarMaker(), nil
	case "carModel":
		return faker.CarModel(), nil
	case "carTransmissionType":
		return faker.CarTransmissionType(), nil
	case "carType":
		return faker.CarType(), nil
	case "cat":
		return faker.Cat(), nil
	case "celebrityActor":
		return faker.CelebrityActor(), nil
	case "celebrityBusiness":
		return faker.CelebrityBusiness(), nil
	case "celebritySport":
		return faker.CelebritySport(), nil
	case "chromeUserAgent":
		return faker.ChromeUserAgent(), nil
	case "city":
		return faker.City(), nil
	case "color":
		return faker.Color(), nil
	case "comment":
		return faker.Comment(), nil
	case "company":
		return faker.Company(), nil
	case "companySuffix":
		return faker.CompanySuffix(), nil
	case "connective":
		return faker.Connective(), nil
	case "connectiveCasual":
		return faker.ConnectiveCasual(), nil
	case "connectiveComparative":
		return faker.ConnectiveComparative(), nil
	case "connectiveComplaint":
		return faker.ConnectiveComplaint(), nil
	case "connectiveExamplify":
		return faker.ConnectiveExamplify(), nil
	case "connectiveListing":
		return faker.ConnectiveListing(), nil
	case "connectiveTime":
		return faker.ConnectiveTime(), nil
	case "country":
		return faker.Country(), nil
	case "countryAbr":
		return faker.CountryAbr(), nil
	case "creditCardCvv":
		return faker.CreditCardCvv(), nil
	case "creditCardExp":
		return faker.CreditCardExp(), nil
	case "creditCardType":
		return faker.CreditCardType(), nil
	case "currencyLong":
		return faker.CurrencyLong(), nil
	case "currencyShort":
		return faker.CurrencyShort(), nil
	case "cusip":
		return faker.Cusip(), nil
	case "dessert":
		return faker.Dessert(), nil
	case "digit":
		return faker.Digit(), nil
	case "dinner":
		return faker.Dinner(), nil
	case "dog":
		return faker.Dog(), nil
	case "domainName":
		return faker.DomainName(), nil
	case "domainSuffix":
		return faker.DomainSuffix(), nil
	case "drink":
		return faker.Drink(), nil
	case "email":
		return faker.Email(), nil
	case "emoji":
		return faker.Emoji(), nil
	case "emojiAlias":
		return faker.EmojiAlias(), nil
	case "emojiCategory":
		return faker.EmojiCategory(), nil
	case "emojiDescription":
		return faker.EmojiDescription(), nil
	case "emojiTag":
		return faker.EmojiTag(), nil
	case "farmAnimal":
		return faker.FarmAnimal(), nil
	case "fileExtension":
		return faker.FileExtension(), nil
	case "fileMimeType":
		return faker.FileMimeType(), nil
	case "firefoxUserAgent":
		return faker.FirefoxUserAgent(), nil
	case "firstName":
		return faker.FirstName(), nil
	case "flipACoin":
		return faker.FlipACoin(), nil
	case "fruit":
		return faker.Fruit(), nil
	case "gamertag":
		return faker.Gamertag(), nil
	case "gender":
		return faker.Gender(), nil
	case "hTTPMethod":
		return faker.HTTPMethod(), nil
	case "hTTPVersion":
		return faker.HTTPVersion(), nil
	case "hackerAbbreviation":
		return faker.HackerAbbreviation(), nil
	case "hackerAdjective":
		return faker.HackerAdjective(), nil
	case "hackerNoun":
		return faker.HackerNoun(), nil
	case "hackerPhrase":
		return faker.HackerPhrase(), nil
	case "hackerVerb":
		return faker.HackerVerb(), nil
	case "hackeringVerb":
		return faker.HackeringVerb(), nil
	case "hexColor":
		return faker.HexColor(), nil
	case "hipsterWord":
		return faker.HipsterWord(), nil
	case "hobby":
		return faker.Hobby(), nil
	case "iPv4Address":
		return faker.IPv4Address(), nil
	case "iPv6Address":
		return faker.IPv6Address(), nil
	case "inputName":
		return faker.InputName(), nil
	case "interjection":
		return faker.Interjection(), nil
	case "isin":
		return faker.Isin(), nil
	case "jobDescriptor":
		return faker.JobDescriptor(), nil
	case "jobLevel":
		return faker.JobLevel(), nil
	case "jobTitle":
		return faker.JobTitle(), nil
	case "language":
		return faker.Language(), nil
	case "languageAbbreviation":
		return faker.LanguageAbbreviation(), nil
	case "languageBCP":
		return faker.LanguageBCP(), nil
	case "lastName":
		return faker.LastName(), nil
	case "letter":
		return faker.Letter(), nil
	case "loremIpsumWord":
		return faker.LoremIpsumWord(), nil
	case "lunch":
		return faker.Lunch(), nil
	case "macAddress":
		return faker.MacAddress(), nil
	case "middleName":
		return faker.MiddleName(), nil
	case "minecraftAnimal":
		return faker.MinecraftAnimal(), nil
	case "minecraftArmorPart":
		return faker.MinecraftArmorPart(), nil
	case "minecraftArmorTier":
		return faker.MinecraftArmorTier(), nil
	case "minecraftBiome":
		return faker.MinecraftBiome(), nil
	case "minecraftDye":
		return faker.MinecraftDye(), nil
	case "minecraftFood":
		return faker.MinecraftFood(), nil
	case "minecraftMobBoss":
		return faker.MinecraftMobBoss(), nil
	case "minecraftMobHostile":
		return faker.MinecraftMobHostile(), nil
	case "minecraftMobNeutral":
		return faker.MinecraftMobNeutral(), nil
	case "minecraftMobPassive":
		return faker.MinecraftMobPassive(), nil
	case "minecraftOre":
		return faker.MinecraftOre(), nil
	case "minecraftTool":
		return faker.MinecraftTool(), nil
	case "minecraftVillagerJob":
		return faker.MinecraftVillagerJob(), nil
	case "minecraftVillagerLevel":
		return faker.MinecraftVillagerLevel(), nil
	case "minecraftVillagerStation":
		return faker.MinecraftVillagerStation(), nil
	case "minecraftWeapon":
		return faker.MinecraftWeapon(), nil
	case "minecraftWeather":
		return faker.MinecraftWeather(), nil
	case "minecraftWood":
		return faker.MinecraftWood(), nil
	case "monthString":
		return faker.MonthString(), nil
	case "movieGenre":
		return faker.MovieGenre(), nil
	case "movieName":
		return faker.MovieName(), nil
	case "name":
		return faker.Name(), nil
	case "namePrefix":
		return faker.NamePrefix(), nil
	case "nameSuffix":
		return faker.NameSuffix(), nil
	case "noun":
		return faker.Noun(), nil
	case "nounAbstract":
		return faker.NounAbstract(), nil
	case "nounCollectiveAnimal":
		return faker.NounCollectiveAnimal(), nil
	case "nounCollectivePeople":
		return faker.NounCollectivePeople(), nil
	case "nounCollectiveThing":
		return faker.NounCollectiveThing(), nil
	case "nounCommon":
		return faker.NounCommon(), nil
	case "nounConcrete":
		return faker.NounConcrete(), nil
	case "nounCountable":
		return faker.NounCountable(), nil
	case "nounDeterminer":
		return faker.NounDeterminer(), nil
	case "nounProper":
		return faker.NounProper(), nil
	case "nounUncountable":
		return faker.NounUncountable(), nil
	case "operaUserAgent":
		return faker.OperaUserAgent(), nil
	case "petName":
		return faker.PetName(), nil
	case "phone":
		return faker.Phone(), nil
	case "phoneFormatted":
		return faker.PhoneFormatted(), nil
	case "phrase":
		return faker.Phrase(), nil
	case "phraseAdverb":
		return faker.PhraseAdverb(), nil
	case "phraseNoun":
		return faker.PhraseNoun(), nil
	case "phrasePreposition":
		return faker.PhrasePreposition(), nil
	case "phraseVerb":
		return faker.PhraseVerb(), nil
	case "preposition":
		return faker.Preposition(), nil
	case "prepositionCompound":
		return faker.PrepositionCompound(), nil
	case "prepositionDouble":
		return faker.PrepositionDouble(), nil
	case "prepositionSimple":
		return faker.PrepositionSimple(), nil
	case "productCategory":
		return faker.ProductCategory(), nil
	case "productDescription":
		return faker.ProductDescription(), nil
	case "productFeature":
		return faker.ProductFeature(), nil
	case "productMaterial":
		return faker.ProductMaterial(), nil
	case "productName":
		return faker.ProductName(), nil
	case "productUPC":
		return faker.ProductUPC(), nil
	case "programmingLanguage":
		return faker.ProgrammingLanguage(), nil
	case "pronoun":
		return faker.Pronoun(), nil
	case "pronounDemonstrative":
		return faker.PronounDemonstrative(), nil
	case "pronounIndefinite":
		return faker.PronounIndefinite(), nil
	case "pronounInterrogative":
		return faker.PronounInterrogative(), nil
	case "pronounObject":
		return faker.PronounObject(), nil
	case "pronounPersonal":
		return faker.PronounPersonal(), nil
	case "pronounPossessive":
		return faker.PronounPossessive(), nil
	case "pronounReflective":
		return faker.PronounReflective(), nil
	case "pronounRelative":
		return faker.PronounRelative(), nil
	case "question":
		return faker.Question(), nil
	case "quote":
		return faker.Quote(), nil
	case "sSN":
		return faker.SSN(), nil
	case "safariUserAgent":
		return faker.SafariUserAgent(), nil
	case "safeColor":
		return faker.SafeColor(), nil
	case "school":
		return faker.School(), nil
	case "sentenceSimple":
		return faker.SentenceSimple(), nil
	case "slogan":
		return faker.Slogan(), nil
	case "snack":
		return faker.Snack(), nil
	case "state":
		return faker.State(), nil
	case "stateAbr":
		return faker.StateAbr(), nil
	case "street":
		return faker.Street(), nil
	case "streetName":
		return faker.StreetName(), nil
	case "streetNumber":
		return faker.StreetNumber(), nil
	case "streetPrefix":
		return faker.StreetPrefix(), nil
	case "streetSuffix":
		return faker.StreetSuffix(), nil
	case "timeZone":
		return faker.TimeZone(), nil
	case "timeZoneAbv":
		return faker.TimeZoneAbv(), nil
	case "timeZoneFull":
		return faker.TimeZoneFull(), nil
	case "timeZoneRegion":
		return faker.TimeZoneRegion(), nil
	case "uRL":
		return faker.URL(), nil
	case "uUID":
		return faker.UUID(), nil
	case "userAgent":
		return faker.UserAgent(), nil
	case "username":
		return faker.Username(), nil
	case "vegetable":
		return faker.Vegetable(), nil
	case "verb":
		return faker.Verb(), nil
	case "verbAction":
		return faker.VerbAction(), nil
	case "verbHelping":
		return faker.VerbHelping(), nil
	case "verbIntransitive":
		return faker.VerbIntransitive(), nil
	case "verbLinking":
		return faker.VerbLinking(), nil
	case "verbTransitive":
		return faker.VerbTransitive(), nil
	case "vowel":
		return faker.Vowel(), nil
	case "weekDay":
		return faker.WeekDay(), nil
	case "word":
		return faker.Word(), nil
	case "zip":
		return faker.Zip(), nil
	default:
		return "", fmt.Errorf("gofakeit method not found: %s", method)
	}
}
