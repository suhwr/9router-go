package dashboard

import (
	"net/url"
	"strings"
)

// appNameSettingKey and appLogoUrlSettingKey are the white-label keys the
// Settings > White-label branding card writes and the sidebar reads. The
// sidebar renders whatever they hold, so the stored shape is kept honest here
// rather than trusted from the client.
const (
	appNameSettingKey              = "appName"
	appSubtitleSettingKey          = "appSubtitle"
	appLogoUrlSettingKey           = "appLogoUrl"
	faviconUrlSettingKey           = "faviconUrl"
	loginTitleSettingKey           = "loginTitle"
	loginSubtitleSettingKey        = "loginSubtitle"
	loginBgUrlSettingKey           = "loginBgUrl"
	loginBgBlurSettingKey          = "loginBgBlur"
	loginHideDefaultPasswordKey    = "loginHideDefaultPasswordHint"
	hideBrandFooterSettingKey      = "hideBrandFooter"
	footerTextSettingKey           = "footerText"
)

// maxAppNameRunes caps the stored product name. The sidebar renders it inside a
// fixed-width brand block next to a 36px logo tile, so a longer string would
// either wrap the header or push the version line out of the aside.
const maxAppNameRunes = 60

// maxAppLogoURLLen caps the stored logo URL. A logo is a URL, not an asset:
// the sidebar points an <img> at it, so the value has to stay short enough to
// sit in a settings row and a JSON response.
const maxAppLogoURLLen = 500

// normalizeAppName coerces a client payload into the stored product name:
// trimmed, and capped at maxAppNameRunes. The second result reports whether the
// payload was a string at all, so a number or a list clears the setting instead
// of being stored as something the sidebar would have to defend against.
func normalizeAppName(value any) (string, bool) {
	name, ok := value.(string)
	if !ok {
		return "", false
	}
	name = strings.TrimSpace(name)
	if len([]rune(name)) > maxAppNameRunes {
		name = string([]rune(name)[:maxAppNameRunes])
	}
	return name, true
}

// normalizeAppLogoUrl coerces a client payload into the stored logo URL:
// trimmed, capped, and restricted to a form the sidebar can actually render.
//
// An https:// URL is the normal case. A local absolute path ("/logo.png") is
// accepted too, because an operator serving the dashboard behind a reverse
// proxy may prefer to host the asset themselves; the scheme-less form is what
// makes that distinguishable from a remote URL. A protocol-relative "//host"
// is refused: it silently re-points the request at whatever host the operator
// typed, which is not the asset they meant to serve.
//
// An empty value clears the logo, which is how the default favicon comes back.
func normalizeAppLogoUrl(value any) (string, bool) {
	logo, ok := value.(string)
	if !ok {
		return "", false
	}
	logo = strings.TrimSpace(logo)
	if len(logo) > maxAppLogoURLLen {
		return "", false
	}
	if logo == "" {
		return "", true
	}
	parsed, err := url.Parse(logo)
	if err != nil {
		return "", false
	}
	isLocalPath := parsed.Scheme == "" && strings.HasPrefix(logo, "/") && !strings.HasPrefix(logo, "//")
	if parsed.Scheme != "https" && !isLocalPath {
		return "", false
	}
	return logo, true
}

func normalizeGenericText(value any, maxRunes int) (string, bool) {
	s, ok := value.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if len([]rune(s)) > maxRunes {
		s = string([]rune(s)[:maxRunes])
	}
	return s, true
}

func normalizeBoolVal(value any) (bool, bool) {
	if b, ok := value.(bool); ok {
		return b, true
	}
	return false, false
}

// applyWhitelabelUpdate normalizes branding keys in HandleUpdateSettings' payload.
func applyWhitelabelUpdate(updates map[string]any) {
	if value, ok := updates[appNameSettingKey]; ok {
		name, valid := normalizeAppName(value)
		if !valid {
			updates[appNameSettingKey] = nil
		} else {
			updates[appNameSettingKey] = name
		}
	}
	if value, ok := updates[appSubtitleSettingKey]; ok {
		sub, valid := normalizeGenericText(value, 100)
		if !valid {
			updates[appSubtitleSettingKey] = nil
		} else {
			updates[appSubtitleSettingKey] = sub
		}
	}
	if value, ok := updates[appLogoUrlSettingKey]; ok {
		logo, valid := normalizeAppLogoUrl(value)
		if !valid {
			updates[appLogoUrlSettingKey] = nil
		} else {
			updates[appLogoUrlSettingKey] = logo
		}
	}
	if value, ok := updates[faviconUrlSettingKey]; ok {
		fav, valid := normalizeAppLogoUrl(value)
		if !valid {
			updates[faviconUrlSettingKey] = nil
		} else {
			updates[faviconUrlSettingKey] = fav
		}
	}
	if value, ok := updates[loginTitleSettingKey]; ok {
		title, valid := normalizeGenericText(value, 60)
		if !valid {
			updates[loginTitleSettingKey] = nil
		} else {
			updates[loginTitleSettingKey] = title
		}
	}
	if value, ok := updates[loginSubtitleSettingKey]; ok {
		sub, valid := normalizeGenericText(value, 160)
		if !valid {
			updates[loginSubtitleSettingKey] = nil
		} else {
			updates[loginSubtitleSettingKey] = sub
		}
	}
	if value, ok := updates[loginBgUrlSettingKey]; ok {
		bg, valid := normalizeAppLogoUrl(value)
		if !valid {
			updates[loginBgUrlSettingKey] = nil
		} else {
			updates[loginBgUrlSettingKey] = bg
		}
	}
	if value, ok := updates[loginBgBlurSettingKey]; ok {
		if b, valid := normalizeBoolVal(value); valid {
			updates[loginBgBlurSettingKey] = b
		} else {
			updates[loginBgBlurSettingKey] = nil
		}
	}
	if value, ok := updates[loginHideDefaultPasswordKey]; ok {
		if b, valid := normalizeBoolVal(value); valid {
			updates[loginHideDefaultPasswordKey] = b
		} else {
			updates[loginHideDefaultPasswordKey] = nil
		}
	}
	if value, ok := updates[hideBrandFooterSettingKey]; ok {
		if b, valid := normalizeBoolVal(value); valid {
			updates[hideBrandFooterSettingKey] = b
		} else {
			updates[hideBrandFooterSettingKey] = nil
		}
	}
	if value, ok := updates[footerTextSettingKey]; ok {
		txt, valid := normalizeGenericText(value, 160)
		if !valid {
			updates[footerTextSettingKey] = nil
		} else {
			updates[footerTextSettingKey] = txt
		}
	}
}

// sanitizeWhitelabel re-reads the stored values through the same normalizers.
func sanitizeWhitelabel(settings map[string]any) {
	if value, ok := settings[appNameSettingKey]; ok {
		name, valid := normalizeAppName(value)
		if !valid || name == "" {
			delete(settings, appNameSettingKey)
		} else {
			settings[appNameSettingKey] = name
		}
	}
	if value, ok := settings[appSubtitleSettingKey]; ok {
		sub, valid := normalizeGenericText(value, 100)
		if !valid || sub == "" {
			delete(settings, appSubtitleSettingKey)
		} else {
			settings[appSubtitleSettingKey] = sub
		}
	}
	if value, ok := settings[appLogoUrlSettingKey]; ok {
		logo, valid := normalizeAppLogoUrl(value)
		if !valid || logo == "" {
			delete(settings, appLogoUrlSettingKey)
		} else {
			settings[appLogoUrlSettingKey] = logo
		}
	}
	if value, ok := settings[faviconUrlSettingKey]; ok {
		fav, valid := normalizeAppLogoUrl(value)
		if !valid || fav == "" {
			delete(settings, faviconUrlSettingKey)
		} else {
			settings[faviconUrlSettingKey] = fav
		}
	}
	if value, ok := settings[loginTitleSettingKey]; ok {
		title, valid := normalizeGenericText(value, 60)
		if !valid || title == "" {
			delete(settings, loginTitleSettingKey)
		} else {
			settings[loginTitleSettingKey] = title
		}
	}
	if value, ok := settings[loginSubtitleSettingKey]; ok {
		sub, valid := normalizeGenericText(value, 160)
		if !valid || sub == "" {
			delete(settings, loginSubtitleSettingKey)
		} else {
			settings[loginSubtitleSettingKey] = sub
		}
	}
	if value, ok := settings[loginBgUrlSettingKey]; ok {
		bg, valid := normalizeAppLogoUrl(value)
		if !valid || bg == "" {
			delete(settings, loginBgUrlSettingKey)
		} else {
			settings[loginBgUrlSettingKey] = bg
		}
	}
	if value, ok := settings[loginBgBlurSettingKey]; ok {
		if b, valid := normalizeBoolVal(value); !valid {
			delete(settings, loginBgBlurSettingKey)
		} else {
			settings[loginBgBlurSettingKey] = b
		}
	}
	if value, ok := settings[loginHideDefaultPasswordKey]; ok {
		if b, valid := normalizeBoolVal(value); !valid {
			delete(settings, loginHideDefaultPasswordKey)
		} else {
			settings[loginHideDefaultPasswordKey] = b
		}
	}
	if value, ok := settings[hideBrandFooterSettingKey]; ok {
		if b, valid := normalizeBoolVal(value); !valid {
			delete(settings, hideBrandFooterSettingKey)
		} else {
			settings[hideBrandFooterSettingKey] = b
		}
	}
	if value, ok := settings[footerTextSettingKey]; ok {
		txt, valid := normalizeGenericText(value, 160)
		if !valid || txt == "" {
			delete(settings, footerTextSettingKey)
		} else {
			settings[footerTextSettingKey] = txt
		}
	}
}
