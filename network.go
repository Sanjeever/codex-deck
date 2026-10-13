package main

import "codex-deck/internal/network"

func (a *App) SaveNetwork(config network.Config) (network.Config, error) {
	s, err := a.backend()
	if err != nil {
		return network.Config{}, err
	}
	if err := s.SaveNetwork(config); err != nil {
		return network.Config{}, err
	}
	return s.GetState().Network, nil
}

func (a *App) TestNetwork(config network.Config) error {
	if _, err := a.backend(); err != nil {
		return err
	}
	return network.Probe(a.ctx, config)
}
