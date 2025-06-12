package conversion

import (
	"fmt"
	"strings"
	"time"

	"github.com/bizflycloud/gobizfly"
)

// Instance represents a BizflyCloud instance with simplified fields
type Instance struct {
	ID        string
	Name      string
	Status    string
	Region    string
	Zone      string
	Flavor    string
	IPAddress string
	IsSpot    bool
	Tags      []string
	CreatedAt time.Time
}

// ConvertGobizflyServerToInstance converts gobizfly.Server to Instance
func ConvertGobizflyServerToInstance(server *gobizfly.Server, region string) *Instance {
	if server == nil {
		return nil
	}

	zone := server.AvailabilityZone
	if zone == "" {
		zone = fmt.Sprintf("%s-a", region)
	}

	ipAddress := ""
	if len(server.IPAddresses.WanV4Addresses) > 0 {
		ipAddress = server.IPAddresses.WanV4Addresses[0].Address
	} else if len(server.IPAddresses.LanAddresses) > 0 {
		ipAddress = server.IPAddresses.LanAddresses[0].Address
	}

	var createdAt time.Time
	if server.CreatedAt != "" {
		formats := []string{
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			time.RFC3339,
		}

		for _, format := range formats {
			if parsed, err := time.Parse(format, server.CreatedAt); err == nil {
				createdAt = parsed
				break
			}
		}

		if createdAt.IsZero() {
			createdAt = time.Now()
		}
	} else {
		createdAt = time.Now()
	}

	var tags []string
	for key, value := range server.Metadata {
		tags = append(tags, fmt.Sprintf("%s=%s", key, value))
	}

	return &Instance{
		ID:        server.ID,
		Name:      server.Name,
		Status:    server.Status,
		Region:    region,
		Zone:      zone,
		Flavor:    server.Flavor.Name,
		IPAddress: ipAddress,
		IsSpot:    false,
		Tags:      tags,
		CreatedAt: createdAt,
	}
}

// CategorizeFlavor determines the flavor category based on flavor name
func CategorizeFlavor(flavorName string) string {
	switch {
	case strings.HasPrefix(flavorName, "nix."):
		return "premium"
	case strings.HasSuffix(flavorName, "_basic"):
		return "basic"
	case strings.HasSuffix(flavorName, "_enterprise"):
		return "enterprise"
	case strings.HasSuffix(flavorName, "_dedicated"):
		return "dedicated"
	default:
		return "premium"
	}
}