package httpapi

import (
	"context"
	"errors"
	"net/http"

	"sortwise/internal/ai"
	"sortwise/internal/model"
	"sortwise/internal/store"
)

const (
	minAnalyzedForCategoryProposals = 20
	proposalTagLimit                = 200
	proposalSummaryLimit            = 40
)

// completer returns the selected provider as a JSON completer, or writes the
// error response and returns nil.
func (s *Server) completer(w http.ResponseWriter, r *http.Request) ai.JSONCompleter {
	providerName, key := s.readyProvider(r.Context())
	if key == "" {
		writeError(w, 400, "ai_unconfigured", "Add a "+ai.ProviderName(providerName)+" API key in Settings first")
		return nil
	}
	factory := s.providers[providerName]
	if factory == nil {
		writeError(w, 400, "ai_unconfigured", "The selected AI provider is unavailable")
		return nil
	}
	provider, err := factory(r.Context(), key)
	if err != nil {
		writeError(w, 502, "ai_connection_failed", "Could not create the AI client")
		return nil
	}
	completer, ok := provider.(ai.JSONCompleter)
	if !ok {
		writeError(w, 400, "ai_unsupported", "The selected AI provider cannot suggest structure")
		return nil
	}
	return completer
}

func (s *Server) proposeCategories(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.store.TaxonomySnapshot(r.Context(), proposalTagLimit, proposalSummaryLimit)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not read the library")
		return
	}
	if snapshot.Analyzed < minAnalyzedForCategoryProposals {
		writeError(w, 409, "not_enough_data", "Analyze at least 20 bookmarks first so suggestions reflect your library")
		return
	}
	completer := s.completer(w, r)
	if completer == nil {
		return
	}
	proposals, err := ai.ProposeCategories(r.Context(), completer, snapshot)
	if err != nil {
		writeError(w, 502, "ai_failed", "Could not get suggestions: "+err.Error())
		return
	}
	payloads := make([]any, 0, len(proposals))
	for _, proposal := range proposals {
		payloads = append(payloads, proposal)
	}
	s.saveAndListProposals(w, r.Context(), "category", payloads)
}

func (s *Server) proposeMerges(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.TagStats(r.Context(), 400)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not read tags")
		return
	}
	merges := ai.ObviousMerges(tags)
	// The model pass is optional: spelling duplicates are found without it.
	if _, key := s.readyProvider(r.Context()); key != "" && len(tags) >= 10 {
		completer := s.completer(w, r)
		if completer == nil {
			return
		}
		suggested, err := ai.ProposeMerges(r.Context(), completer, tags)
		if err != nil {
			writeError(w, 502, "ai_failed", "Could not get suggestions: "+err.Error())
			return
		}
		seen := map[string]bool{}
		for _, merge := range merges {
			seen[merge.Source] = true
		}
		for _, merge := range suggested {
			if !seen[merge.Source] && !seen[merge.Target] {
				merges = append(merges, merge)
				seen[merge.Source] = true
			}
		}
	}
	payloads := make([]any, 0, len(merges))
	for _, merge := range merges {
		payloads = append(payloads, merge)
	}
	s.saveAndListProposals(w, r.Context(), "merge", payloads)
}

func (s *Server) saveAndListProposals(w http.ResponseWriter, ctx context.Context, kind string, payloads []any) {
	if err := s.store.ReplaceProposals(ctx, kind, payloads); err != nil {
		writeError(w, 500, "storage_error", "Could not save suggestions")
		return
	}
	s.writeProposals(w, ctx)
}

func (s *Server) listProposals(w http.ResponseWriter, r *http.Request) {
	s.writeProposals(w, r.Context())
}

func (s *Server) writeProposals(w http.ResponseWriter, ctx context.Context) {
	items, err := s.store.ListProposals(ctx)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load suggestions")
		return
	}
	if items == nil {
		items = []model.TaxonomyProposal{}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) acceptProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	if err := s.store.AcceptProposal(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrProposalNotFound) {
			writeError(w, 404, "not_found", "This suggestion was already handled")
			return
		}
		writeError(w, 422, "proposal_failed", err.Error())
		return
	}
	s.writeProposals(w, r.Context())
}

func (s *Server) dismissProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	if err := s.store.DismissProposal(r.Context(), id); err != nil {
		writeError(w, 404, "not_found", "This suggestion was already handled")
		return
	}
	s.writeProposals(w, r.Context())
}
