package main

import (
	"codex-deck/internal/sessions"
	"fmt"
)

// Session bindings wait for initialization but do not depend on account loading,
// authentication, automatic-refresh pausing, or deck:state broadcasts.
func (a *App) sessionReader() (*sessions.Service, error) {
	<-a.ready
	if a.sessions == nil {
		return nil, fmt.Errorf("会话读取服务未初始化")
	}
	return a.sessions, nil
}

func (a *App) ListSessions(query sessions.ListQuery) (sessions.ListPage, error) {
	s, err := a.sessionReader()
	if err != nil {
		return sessions.ListPage{}, err
	}
	return s.List(query)
}
func (a *App) ReadSession(query sessions.ReadQuery) (sessions.ReadPage, error) {
	s, err := a.sessionReader()
	if err != nil {
		return sessions.ReadPage{}, err
	}
	return s.Read(query)
}
func (a *App) FindSessionMatches(query sessions.MatchQuery) (sessions.MatchPage, error) {
	s, err := a.sessionReader()
	if err != nil {
		return sessions.MatchPage{}, err
	}
	return s.Find(query)
}
func (a *App) ReadSessionText(query sessions.TextQuery) (sessions.TextPage, error) {
	s, err := a.sessionReader()
	if err != nil {
		return sessions.TextPage{}, err
	}
	return s.ReadText(query)
}
