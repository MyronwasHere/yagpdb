package serverstats

import (
	"github.com/jonas747/yagpdb/common"
	"github.com/jonas747/yagpdb/web"
	"goji.io/pat"
	"golang.org/x/net/context"
	"html/template"
	"log"
	"net/http"
)

func (p *Plugin) InitWeb() {
	web.Templates = template.Must(web.Templates.ParseFiles("templates/plugins/serverstats.html"))
	web.CPMux.HandleC(pat.Get("/cp/:server/stats"), web.RenderHandler(publicHandler(HandleStatsHtml, false), "cp_serverstats"))
	web.CPMux.HandleC(pat.Get("/cp/:server/stats/"), web.RenderHandler(publicHandler(HandleStatsHtml, false), "cp_serverstats"))

	web.CPMux.HandleC(pat.Post("/cp/:server/stats/settings"), web.RenderHandler(HandleStatsSettings, "cp_serverstats"))
	web.CPMux.HandleC(pat.Get("/cp/:server/stats/full"), web.APIHandler(publicHandler(HandleStatsJson, false)))

	// Public
	web.RootMux.HandleC(pat.Get("/public/:server/stats"), web.RenderHandler(publicHandler(HandleStatsHtml, true), "cp_serverstats"))
	web.RootMux.HandleC(pat.Get("/public/:server/stats/full"), web.APIHandler(publicHandler(HandleStatsJson, true)))
}

type publicHandlerFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, publicAccess bool) interface{}

func publicHandler(inner publicHandlerFunc, public bool) web.CustomHandlerFunc {
	mw := func(ctx context.Context, w http.ResponseWriter, r *http.Request) interface{} {
		return inner(ctx, w, r, public)
	}

	return mw
}

// Somewhat dirty - should clean up this mess sometime
func HandleStatsHtml(ctx context.Context, w http.ResponseWriter, r *http.Request, isPublicAccess bool) interface{} {
	client := web.RedisClientFromContext(ctx)

	var guildID string
	var guildName string
	templateData := make(map[string]interface{})

	publicEnabled, _ := client.Cmd("GET", "stats_settings_public:"+guildID).Bool()

	if !isPublicAccess {
		_, activeGuild, t := web.GetBaseCPContextData(ctx)
		templateData = t
		guildName = activeGuild.Name
	} else {
		if !publicEnabled {
			return templateData
		}

		guild, err := common.GetGuild(client, guildID)
		if web.CheckErr(templateData, err) {
			return templateData
		}

		guildName = guild.Name

		templateData["public_guild_id"] = guildID
	}

	templateData["public"] = publicEnabled
	templateData["guild_name"] = guildName

	return templateData
}

func HandleStatsSettings(ctx context.Context, w http.ResponseWriter, r *http.Request) interface{} {
	client, activeGuild, templateData := web.GetBaseCPContextData(ctx)
	templateData["current_page"] = "serverstats"

	public := r.FormValue("public") == "on"

	current, _ := client.Cmd("GET", "stats_settings_public:"+activeGuild.ID).Bool()
	err := client.Cmd("SET", "stats_settings_public:"+activeGuild.ID, public).Err

	if err != nil {
		log.Println("Error saving stats setting", err)
		templateData["public"] = current
	} else {
		templateData["public"] = public
	}

	return templateData
}

func HandleStatsJson(ctx context.Context, w http.ResponseWriter, r *http.Request, isPublicAccess bool) interface{} {
	client := web.RedisClientFromContext(ctx)

	var guildID string

	if !isPublicAccess {
		_, activeGuild, _ := web.GetBaseCPContextData(ctx)
		guildID = activeGuild.ID
	} else {
		guildID = pat.Param(ctx, "server")
		public, _ := client.Cmd("GET", "stats_settings_public:"+guildID).Bool()
		if !public {
			w.WriteHeader(http.StatusUnauthorized)
			return nil
		}
	}

	stats, err := RetrieveFullStats(client, guildID)
	if err != nil {
		log.Println("Failed retrieving stats", err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil
	}

	return stats
}
