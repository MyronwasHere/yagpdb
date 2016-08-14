package reddit

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/yagpdb/common"
	"github.com/jonas747/yagpdb/web"
	"goji.io"
	"goji.io/pat"
	"golang.org/x/net/context"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type CtxKey int

const (
	CurrentConfig CtxKey = iota
)

func (p *Plugin) InitWeb() {
	web.Templates = template.Must(web.Templates.ParseFiles("templates/plugins/reddit.html"))

	web.CPMux.HandleC(pat.Get("/cp/:server/reddit"), baseData(web.RenderHandler(HandleReddit, "cp_reddit")))
	web.CPMux.HandleC(pat.Get("/cp/:server/reddit/"), baseData(web.RenderHandler(HandleReddit, "cp_reddit")))

	// If only html allowed patch and delete.. if only
	web.CPMux.HandleC(pat.Post("/cp/:server/reddit"), baseData(web.RenderHandler(HandleNew, "cp_reddit")))
	web.CPMux.HandleC(pat.Post("/cp/:server/reddit/:item/update"), baseData(web.RenderHandler(HandleModify, "cp_reddit")))
	web.CPMux.HandleC(pat.Post("/cp/:server/reddit/:item/delete"), baseData(web.RenderHandler(HandleRemove, "cp_reddit")))
}

// Adds the current config to the context
func baseData(inner goji.Handler) goji.Handler {
	mw := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		client, activeGuild, templateData := web.GetBaseCPContextData(ctx)

		currentConfig, err := GetConfig(client, "guild_subreddit_watch:"+activeGuild.ID)
		if err != nil {
			templateData.AddAlerts(web.ErrorAlert("Failed retrieving current config, conact support", err))
			log.Println("Failed retrieving config", activeGuild.ID, err)
			web.LogIgnoreErr(web.Templates.ExecuteTemplate(w, "cp_reddit", templateData))
			return
		}
		templateData["current_page"] = "reddit"
		inner.ServeHTTPC(context.WithValue(ctx, CurrentConfig, currentConfig), w, r)

	}
	return goji.HandlerFunc(mw)

}

func HandleReddit(ctx context.Context, w http.ResponseWriter, r *http.Request) interface{} {
	_, _, templateData := web.GetBaseCPContextData(ctx)

	currentConfig := ctx.Value(CurrentConfig).([]*SubredditWatchItem)
	templateData["current_config"] = currentConfig

	return templateData
}

func HandleNew(ctx context.Context, w http.ResponseWriter, r *http.Request) interface{} {
	client, activeGuild, templateData := web.GetBaseCPContextData(ctx)

	currentConfig := ctx.Value(CurrentConfig).([]*SubredditWatchItem)
	templateData["current_config"] = currentConfig

	highest := 0
	for _, v := range currentConfig {
		if v.ID > highest {
			highest = v.ID
		}
	}

	if len(currentConfig) > 10 {
		return templateData.AddAlerts(web.ErrorAlert("Max 10 items allowed"))
	}

	channelId, ok := GetChannel(r.FormValue("channel"), activeGuild.ID, templateData)
	if !ok {
		return templateData.AddAlerts(web.ErrorAlert("Unknown channel"))
	}

	watchItem := &SubredditWatchItem{
		Sub:     strings.ToLower(r.FormValue("subreddit")),
		Channel: channelId,
		Guild:   activeGuild.ID,
		ID:      highest + 1,
	}

	err := watchItem.Set(client)
	if web.CheckErr(templateData, err) {
		return templateData
	}

	currentConfig = append(currentConfig, watchItem)
	templateData["current_config"] = currentConfig
	templateData.AddAlerts(web.SucessAlert("Sucessfully added subreddit feed for /r/" + watchItem.Sub))

	// Log
	user := ctx.Value(web.ContextKeyUser).(*discordgo.User)
	common.AddCPLogEntry(client, activeGuild.ID, fmt.Sprintf("%s(%s) Added feed from /r/%s", user.Username, user.ID, r.FormValue("subreddit")))
	return templateData
}

func HandleModify(ctx context.Context, w http.ResponseWriter, r *http.Request) interface{} {
	client, activeGuild, templateData := web.GetBaseCPContextData(ctx)

	currentConfig := ctx.Value(CurrentConfig).([]*SubredditWatchItem)
	templateData["current_config"] = currentConfig

	id := pat.Param(ctx, "item")
	idInt, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return templateData.AddAlerts(web.ErrorAlert("Failed parsing id", err))
	}

	item := FindWatchItem(currentConfig, int(idInt))
	if item == nil {
		return templateData.AddAlerts(web.ErrorAlert("Unknown id"))
	}

	r.ParseForm()
	channel, ok := GetChannel(r.FormValue("channel"), activeGuild.ID, templateData)
	if !ok {
		return templateData.AddAlerts(web.ErrorAlert("Failed retrieving channel"))
	}

	newSub := r.FormValue("subreddit") == item.Sub

	item.Channel = channel

	if !newSub {
		// Pretty simple then
		err = item.Set(client)
	} else {
		err = item.Remove(client)
		if err == nil {
			item.Sub = strings.ToLower(r.FormValue("subreddit"))
			err = item.Set(client)
		}
	}

	if web.CheckErr(templateData, err) {
		return templateData
	}

	templateData.AddAlerts(web.SucessAlert("Sucessfully updated reddit feed! :D"))

	user := ctx.Value(web.ContextKeyUser).(*discordgo.User)
	common.AddCPLogEntry(client, activeGuild.ID, fmt.Sprintf("%s(%s) Modified a feed to /r/%s", user.Username, user.ID, r.FormValue("subreddit")))
	return templateData
}

func HandleRemove(ctx context.Context, w http.ResponseWriter, r *http.Request) interface{} {
	client, activeGuild, templateData := web.GetBaseCPContextData(ctx)

	currentConfig := ctx.Value(CurrentConfig).([]*SubredditWatchItem)
	templateData["current_config"] = currentConfig

	id := pat.Param(ctx, "item")
	idInt, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return templateData.AddAlerts(web.ErrorAlert("Failed parsing id", err))
	}

	// Get tha actual watch item from the config
	item := FindWatchItem(currentConfig, int(idInt))

	if item == nil {
		return templateData.AddAlerts(web.ErrorAlert("Unknown id"))
	}

	err = item.Remove(client)
	if web.CheckErr(templateData, err) {
		return templateData
	}

	templateData.AddAlerts(web.SucessAlert("Sucessfully removed subreddit feed for /r/ :')", item.Sub))

	// Remove it form the displayed list
	for k, c := range currentConfig {
		if c.ID == int(idInt) {
			currentConfig = append(currentConfig[:k], currentConfig[k+1:]...)
		}
	}

	templateData["current_config"] = currentConfig

	user := ctx.Value(web.ContextKeyUser).(*discordgo.User)
	common.AddCPLogEntry(client, activeGuild.ID, fmt.Sprintf("%s(%s) Removed feed from /r/%s", user.Username, user.ID, r.FormValue("subreddit")))
	return templateData
}

// Validates a channel name or id, adds an error message if not found
// Returns true if everythign went okay
func GetChannel(name, guild string, templateData web.TemplateData) (string, bool) {
	if name == "" {
		return guild, true
	}

	channelId, err := web.GetChannelId(name, guild)
	if err != nil {
		templateData.AddAlerts(web.ErrorAlert("Failed getting channel", err))
		log.Println("Failed getting channel", guild, err)
		return "", false
	}
	return channelId, true
}
