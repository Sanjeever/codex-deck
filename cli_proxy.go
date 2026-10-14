package main

import (
	"codex-deck/internal/deck"
	"codex-deck/internal/network"
)

func (a *App) GetCLIProxy() (deck.CLIProxyState, error) {
	s, err := a.backend()
	if err != nil {
		return deck.CLIProxyState{}, err
	}
	return s.GetCLIProxy()
}

func (a *App) SaveCLIProxy(change deck.CLIProxyChange) (deck.CLIProxyState, error) {
	s, err := a.backend()
	if err != nil {
		return deck.CLIProxyState{}, err
	}
	return s.SaveCLIProxy(change)
}

func (a *App) TestCLIProxy(address string) error {
	if _, err := a.backend(); err != nil {
		return err
	}
	config, err := deck.CLIProxyNetwork(address)
	if err != nil {
		return err
	}
	return network.Probe(a.ctx, config)
}
