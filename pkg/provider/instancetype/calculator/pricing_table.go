package calculator

// Hourly on-demand prices (VND, excl. VAT, server powered on) from the Bizfly Cloud
// Server price lists "Chip Intel Gen 2" and "Chip AMD Gen 4", effective 05/09/2025.

// tier is one row of a price list: the price of size cores (or GB of RAM) per hour.
type tier struct {
	size  float64
	price float64
}

// intel2CPU: Intel Gen 2, price of N cores by category.
var intel2CPU = map[string][]tier{
	"basic": {
		{1, 75},
		{2, 150},
		{3, 263},
		{4, 465},
		{6, 1230},
		{8, 1785},
		{12, 3330},
	},
	"premium": {
		{1, 150},
		{2, 488},
		{3, 780},
		{4, 1170},
		{6, 1755},
		{8, 2535},
		{10, 3600},
		{12, 4680},
		{16, 6225},
		{18, 7020},
		{24, 9360},
		{32, 12465},
	},
	"enterprise": {
		{1, 233},
		{2, 780},
		{3, 1245},
		{4, 1868},
		{6, 2805},
		{8, 4050},
		{10, 5760},
		{12, 7485},
		{16, 9975},
		{18, 11220},
		{24, 14955},
		{32, 19935},
		{40, 27420},
		{48, 32910},
		{56, 38385},
		{64, 43868},
		{96, 65805},
	},
	"dedicated": {
		{1, 675},
		{2, 1350},
		{3, 2033},
		{4, 2715},
		{6, 4065},
		{8, 5415},
		{10, 6780},
		{12, 8130},
		{16, 10830},
		{18, 12195},
		{24, 16260},
		{32, 21668},
		{40, 27083},
		{48, 32505},
		{56, 37920},
		{64, 43335},
		{96, 65010},
	},
}

// intel2RAM: Intel Gen 2, price of N GB of RAM by category.
var intel2RAM = map[string][]tier{
	"basic": {
		{1, 128},
		{2, 165},
		{3, 233},
		{4, 518},
		{6, 750},
		{8, 960},
		{12, 1410},
		{16, 1980},
		{24, 3300},
	},
	"premium": {
		{1, 248},
		{2, 405},
		{3, 570},
		{4, 735},
		{6, 1065},
		{8, 1395},
		{10, 1725},
		{12, 2040},
		{16, 2700},
		{20, 3360},
		{24, 4020},
		{32, 5325},
		{48, 7935},
		{64, 11520},
		{96, 15795},
		{128, 21150},
	},
	"enterprise": {
		{1, 398},
		{2, 660},
		{3, 930},
		{4, 1185},
		{6, 1703},
		{8, 2228},
		{10, 2753},
		{12, 3270},
		{16, 4320},
		{20, 5370},
		{24, 6420},
		{32, 8520},
		{48, 12698},
		{64, 18435},
		{96, 25268},
		{128, 33645},
		{192, 55440},
		{256, 73875},
		{512, 147600},
	},
	"dedicated": {
		{1, 293},
		{2, 548},
		{3, 878},
		{4, 1065},
		{6, 1755},
		{8, 2340},
		{10, 2918},
		{12, 3503},
		{16, 4665},
		{20, 5835},
		{24, 7005},
		{32, 9338},
		{48, 14003},
		{64, 18668},
		{96, 28005},
		{128, 37335},
		{192, 56003},
		{256, 74670},
		{512, 149333},
	},
}

// AMD Gen 4 is priced linearly: N cores (or GB) cost N times the unit price. It is
// only sold as premium, enterprise and dedicated.
var amd4CPUPerCore = map[string]float64{
	"premium":    292,
	"enterprise": 396,
	"dedicated":  1167,
}

const amd4RAMPerGB = 177

// diskRate is the hourly price of one GB of disk: first for the first 100 GB, tail
// for every GB after that.
type diskRate struct {
	first float64
	tail  float64
}

// diskRates are the on-demand disk prices by category and disk type. They are the
// same for Intel Gen 2 and AMD Gen 4. Cells left blank in the Intel list are derived
// from the listed ones by ratio:
//   - basic SSD beyond 100 GB: basic SSD x the SSD tail/first ratio of the other tiers;
//   - basic NVMe: NVMe x the basic/premium SSD ratio;
//   - premium NVMe: same as enterprise, like HDD and SSD (and AMD Gen 4 premium).
var diskRates = map[string]map[string]diskRate{
	"basic": {
		"HDD":  {2.0, 1.4},
		"SSD":  {6.2, 6.2 * 8.3 / 12.6},
		"NVME": {19.0 * 6.2 / 12.6, 12.5 * 6.2 / 12.6},
	},
	"premium": {
		"HDD":  {3.6, 1.8},
		"SSD":  {12.6, 8.3},
		"NVME": {19.0, 12.5},
	},
	"enterprise": {
		"HDD":  {3.6, 1.8},
		"SSD":  {12.6, 8.3},
		"NVME": {19.0, 12.5},
	},
	"dedicated": {
		"HDD":  {3.6, 1.8},
		"SSD":  {12.6, 8.3},
		"NVME": {19.0, 12.5},
	},
}
