package main

import (
	"errors"

	"codex-deck/internal/deck"
)

type ConfigurationSaveResult struct {
	Configuration *deck.Configuration `json:"configuration"`
	Section       string              `json:"section"`
	Error         string              `json:"error"`
}

func (a *App) SaveConfiguration(config deck.Configuration) (ConfigurationSaveResult, error) {
	s, err := a.backend()
	if err != nil {
		return ConfigurationSaveResult{}, err
	}
	config, err = deck.ValidateConfiguration(config)
	if err == nil {
		err = a.authorizeNotifications(config.Preferences.LowQuotaNotify, s.GetState().Preferences.LowQuotaNotify)
		if err != nil {
			err = &deck.ConfigurationError{Section: "general", Err: err}
		}
	}
	if err == nil {
		config, err = s.SaveConfiguration(config)
	}
	if err != nil {
		result := ConfigurationSaveResult{Error: err.Error()}
		var fieldError *deck.ConfigurationError
		if errors.As(err, &fieldError) {
			result.Section = fieldError.Section
		}
		return result, nil
	}
	return ConfigurationSaveResult{Configuration: &config}, nil
}
