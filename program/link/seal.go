package link

import (
	"errors"

	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

// Seal freezes external source roots and the finite structural project
// applications they support. Program and target remain authoritative for all
// source and ABI correspondence behind those applications.
func Seal(spec *Spec) (*Link, error) {
	return seal(spec, nil)
}

// sealReplay performs the same child construction as Seal but admits Host
// only through its detached replay contract. It is intentionally private to
// artifact decoding: callers cannot bypass authored Host admission.
func sealReplay(spec *Spec, replay linkhost.ReplaySpec) (*Link, error) {
	return seal(spec, &replay)
}

func seal(spec *Spec, replay *linkhost.ReplaySpec) (*Link, error) {
	if spec == nil {
		return nil, errors.New("link: nil spec")
	}
	if spec.consumed {
		return nil, errors.New("link: consumed spec")
	}
	defer func() { *spec = Spec{consumed: true} }()
	if spec.Target == nil || !spec.Target.ContentID().Available() {
		return nil, errors.New("link: unavailable target contract")
	}
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: spec.Modules, Target: spec.Target})
	if err != nil {
		return nil, err
	}
	project, err := projectDraft.Finalize()
	if err != nil || project == nil {
		return nil, errors.New("link: unavailable project authority")
	}
	boundaryDraft, err := linkboundary.Build(linkboundary.Input{Project: project, Target: spec.Target, EndpointRequests: spec.EndpointRequests})
	if err != nil {
		return nil, err
	}
	boundary, err := boundaryDraft.Finalize()
	if err != nil || boundary == nil {
		return nil, errors.New("link: unavailable boundary authority")
	}
	moduleDraft, err := linkmodule.Build(linkmodule.Input{
		Project:  project,
		Boundary: boundary,
		Spec:     spec.Module,
	})
	if err != nil {
		return nil, err
	}
	module, err := moduleDraft.Finalize()
	if err != nil || module == nil {
		return nil, errors.New("link: unavailable module authority")
	}
	link := &Link{
		// Project is the exact finalized authority consumed by Host selector
		// normalization and every later root derivation. Publish it before the
		// first consumer; it is immutable and never replaced.
		project:  project,
		boundary: boundary,
		module:   module,
	}
	staticDraft, err := linkstatic.Build(linkstatic.Input{Project: project})
	if err != nil {
		return nil, err
	}
	hostInput := linkhost.Input{Project: project, Boundary: boundary, Module: module}
	var hostDraft *linkhost.Draft
	if replay == nil {
		hostInput.Spec = spec.Host
		hostDraft, err = linkhost.Build(hostInput)
	} else {
		if len(spec.Host.ProviderCapabilities) != 0 || len(spec.Host.ProviderCapabilitySeeds) != 0 || len(spec.Host.Exposures) != 0 || len(spec.Host.Members) != 0 {
			return nil, errors.New("link: replay mixed with authored host input")
		}
		hostDraft, err = linkhost.BuildReplay(hostInput, *replay)
	}
	if err != nil {
		return nil, err
	}
	link.host, err = hostDraft.Finalize()
	if err != nil || link.host == nil {
		return nil, err
	}
	link.id = contentID(link, project.Cold(), boundary, module.Cold(), link.host.Cold(), staticDraft.Cold())
	if !link.id.Available() {
		return nil, errors.New("link: unavailable content identity")
	}
	link.static, err = staticDraft.Finalize()
	if err != nil || link.static == nil {
		return nil, errors.New("link: unavailable static identity binding")
	}
	link.semanticReceipt, err = buildSemanticSourceReceipt(link)
	if err != nil {
		return nil, err
	}
	return link, nil
}
