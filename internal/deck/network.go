package deck

import "codex-deck/internal/network"

func (s *Service) SaveNetwork(config network.Config) error {
	config, err := network.Normalize(config)
	if err != nil {
		return err
	}
	route, err := network.Prepare(config)
	if err != nil {
		return err
	}
	s.CancelAutomatic()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.Update(func(d *Data) error { d.Network = &config; return nil }); err != nil {
		return err
	}
	s.HTTP.Apply(route)
	return nil
}
