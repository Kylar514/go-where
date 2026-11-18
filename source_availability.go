package main

import "fmt"

// Startup isolates source failures; editors and launch resolution keep strict
// errors so an unavailable inventory can never masquerade as writable emptiness.
func availableInventoryHosts(c Config) ([]Host, map[string]string) {
	var hosts []Host
	failures := map[string]string{}
	for _, source := range c.Sources {
		if !sourceEnabled(c, source.ID) {
			continue
		}
		one := c
		one.Sources = []InventorySource{source}
		rows, err := inventoryHosts(one)
		if err != nil {
			failures[source.ID] = fmt.Sprintf("%s: %v", source.Name, err)
			continue
		}
		if source.Type == "api" {
			if status := apiSourceAvailability(source); status != "" {
				failures[source.ID] = source.Name + " — " + status
			}
		}
		hosts = append(hosts, rows...)
	}
	return hosts, failures
}
