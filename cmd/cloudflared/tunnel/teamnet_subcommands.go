package tunnel

import (
	"fmt"
	"net"
	"os"
	"text/tabwriter"

	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
	"github.com/cloudflare/cloudflared/teamnet"
	"github.com/pkg/errors"

	"github.com/urfave/cli/v2"
)

func buildRouteIPSubcommand() *cli.Command {
	return &cli.Command{
		Name:      "ip",
		Category:  "Tunnel",
		Usage:     "Configure and query Cloudflare for Teams private routes",
		UsageText: "cloudflared tunnel [--config FILEPATH] route COMMAND [arguments...]",
		Hidden:    true,
		Description: `cloudflared lets you provision private Cloudflare for Teams routes to origins in your corporate
		network, so that you can ensure the only people who can access your private IP subnets are people using a
		corporate device enrolled in Cloudflare for Teams.
		`,
		Subcommands: []*cli.Command{
			{
				Name:      "add",
				Action:    cliutil.ErrorHandler(addRouteCommand),
				Usage:     "Add a new Teamnet route to the table",
				UsageText: "cloudflared tunnel [--config FILEPATH] route ip add [CIDR] [TUNNEL] [COMMENT?]",
				Description: `Add a new Cloudflare for Teams private route from a given tunnel (identified by name or
				UUID) to a given IP network in your private IP space. This route will go through your Gateway rules.`,
			},
			{
				Name:      "show",
				Action:    cliutil.ErrorHandler(showRoutesCommand),
				Usage:     "Show the routing table",
				UsageText: "cloudflared tunnel [--config FILEPATH] route ip show [flags]",
				Description: `Shows all Cloudflare for Teams private routes. Using flags to specify filters means that
				only routes which match that filter get shown.`,
				Flags: teamnet.FilterFlags,
			},
			{
				Name:        "delete",
				Action:      cliutil.ErrorHandler(deleteRouteCommand),
				Usage:       "Delete a row of the routing table",
				UsageText:   "cloudflared tunnel [--config FILEPATH] route ip delete [CIDR]",
				Description: `Deletes the Cloudflare for Teams private route for a given CIDR`,
			},
			{
				Name:        "get",
				Action:      cliutil.ErrorHandler(getRouteByIPCommand),
				Usage:       "Check which row of the routing table matches a given IP",
				UsageText:   "cloudflared tunnel [--config FILEPATH] route ip get [IP]",
				Description: `Checks which row of the routing table matches a given IP. This helps check and validate your config.`,
			},
		},
	}
}

func showRoutesCommand(c *cli.Context) error {
	sc, err := newSubcommandContext(c)
	if err != nil {
		return err
	}

	filter, err := teamnet.NewFromCLI(c)
	if err != nil {
		return errors.Wrap(err, "invalid config for routing filters")
	}

	routes, err := sc.listRoutes(filter)
	if err != nil {
		return err
	}

	if outputFormat := c.String(outputFormatFlag.Name); outputFormat != "" {
		return renderOutput(outputFormat, routes)
	}

	if len(routes) > 0 {
		formatAndPrintRouteList(routes)
	} else {
		fmt.Println("You have no routes, use 'cloudflared tunnel route ip add' to add a route")
	}
	return nil
}

func addRouteCommand(c *cli.Context) error {
	sc, err := newSubcommandContext(c)
	if err != nil {
		return err
	}
	if c.NArg() < 2 {
		return errors.New("You must supply at least 2 arguments, first the network you wish to route (in CIDR form e.g. 1.2.3.4/32) and then the tunnel ID to proxy with")
	}
	args := c.Args()
	_, network, err := net.ParseCIDR(args.Get(0))
	if err != nil {
		return errors.Wrap(err, "Invalid network CIDR")
	}
	if network == nil {
		return errors.New("Invalid network CIDR")
	}
	tunnelRef := args.Get(1)
	tunnelID, err := sc.findID(tunnelRef)
	if err != nil {
		return errors.Wrap(err, "Invalid tunnel")
	}
	comment := ""
	if c.NArg() >= 3 {
		comment = args.Get(2)
	}
	_, err = sc.addRoute(teamnet.NewRoute{
		Comment:  comment,
		Network:  *network,
		TunnelID: tunnelID,
	})
	if err != nil {
		return errors.Wrap(err, "API error")
	}
	fmt.Printf("Successfully added route for %s over tunnel %s\n", network, tunnelID)
	return nil
}

func deleteRouteCommand(c *cli.Context) error {
	sc, err := newSubcommandContext(c)
	if err != nil {
		return err
	}
	if c.NArg() != 1 {
		return errors.New("You must supply exactly one argument, the network whose route you want to delete (in CIDR form e.g. 1.2.3.4/32)")
	}
	_, network, err := net.ParseCIDR(c.Args().First())
	if err != nil {
		return errors.Wrap(err, "Invalid network CIDR")
	}
	if network == nil {
		return errors.New("Invalid network CIDR")
	}
	if err := sc.deleteRoute(*network); err != nil {
		return errors.Wrap(err, "API error")
	}
	fmt.Printf("Successfully deleted route for %s\n", network)
	return nil
}

func getRouteByIPCommand(c *cli.Context) error {
	sc, err := newSubcommandContext(c)
	if err != nil {
		return err
	}
	if c.NArg() != 1 {
		return errors.New("You must supply exactly one argument, an IP whose route will be queried (e.g. 1.2.3.4 or 2001:0db8:::7334)")
	}

	ipInput := c.Args().First()
	ip := net.ParseIP(ipInput)
	if ip == nil {
		return fmt.Errorf("Invalid IP %s", ipInput)
	}
	route, err := sc.getRouteByIP(ip)
	if err != nil {
		return errors.Wrap(err, "API error")
	}
	if route.IsZero() {
		fmt.Printf("No route matches the IP %s\n", ip)
	} else {
		formatAndPrintRouteList([]*teamnet.DetailedRoute{&route})
	}
	return nil
}

func formatAndPrintRouteList(routes []*teamnet.DetailedRoute) {
	const (
		minWidth = 0
		tabWidth = 8
		padding  = 1
		padChar  = ' '
		flags    = 0
	)

	writer := tabwriter.NewWriter(os.Stdout, minWidth, tabWidth, padding, padChar, flags)
	defer writer.Flush()

	// Print column headers with tabbed columns
	_, _ = fmt.Fprintln(writer, "NETWORK\tCOMMENT\tTUNNEL ID\tTUNNEL NAME\tCREATED\tDELETED\t")

	// Loop through routes, create formatted string for each, and print using tabwriter
	for _, route := range routes {
		formattedStr := route.TableString()
		_, _ = fmt.Fprintln(writer, formattedStr)
	}
}
