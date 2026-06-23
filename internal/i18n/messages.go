package i18n

// messages is the translation catalog: message id → language → text.
// German is the canonical source; English mirrors it. Messages with a
// %s/%d verb are formatted by Localizer.T with the call-site arguments.
var messages = map[string]map[Lang]string{
	// ---- Shared chrome & actions ----------------------------------------
	"nav.handbook":  {DE: "📖 Handbuch", EN: "📖 Manual"},
	"nav.settings":  {DE: "⚙ Einstellungen", EN: "⚙ Settings"},
	"nav.start":     {DE: "Start", EN: "Home"},
	"action.login":  {DE: "Anmelden", EN: "Sign in"},
	"action.logout": {DE: "Abmelden", EN: "Sign out"},
	"action.play":   {DE: "Spielen", EN: "Play"},
	"action.back":   {DE: "← Zurück", EN: "← Back"},
	"action.done":   {DE: "Fertig", EN: "Done"},
	"action.cancel": {DE: "Abbrechen", EN: "Cancel"},
	"action.copy":   {DE: "kopieren", EN: "copy"},
	"action.save":   {DE: "Speichern", EN: "Save"},

	// ---- Status & roles -------------------------------------------------
	"status.active":   {DE: "aktiv", EN: "active"},
	"status.inactive": {DE: "inaktiv", EN: "inactive"},
	"color.black":     {DE: "Schwarz", EN: "Black"},
	"color.white":     {DE: "Weiß", EN: "White"},
	"color.black_up":  {DE: "SCHWARZ", EN: "BLACK"},
	"color.white_up":  {DE: "WEISS", EN: "WHITE"},
	"result.win":      {DE: "Sieg", EN: "Win"},
	"result.loss":     {DE: "Niederlage", EN: "Loss"},

	// ---- Table headings -------------------------------------------------
	"th.name":        {DE: "Name", EN: "Name"},
	"th.email":       {DE: "E-Mail", EN: "Email"},
	"th.strength":    {DE: "Spielstärke", EN: "Strength"},
	"th.status":      {DE: "Status", EN: "Status"},
	"th.date":        {DE: "Datum", EN: "Date"},
	"th.time":        {DE: "Zeit", EN: "Time"},
	"th.board":       {DE: "Brett", EN: "Board"},
	"th.handicap":    {DE: "Vorgabe", EN: "Handicap"},
	"th.komi":        {DE: "Komi", EN: "Komi"},
	"th.winner":      {DE: "Sieger", EN: "Winner"},
	"th.color":       {DE: "Farbe", EN: "Color"},
	"th.opponent":    {DE: "Gegner", EN: "Opponent"},
	"th.result":      {DE: "Ergebnis", EN: "Result"},
	"th.games":       {DE: "Partien", EN: "Games"},
	"th.delta_black": {DE: "Δ Schwarz", EN: "Δ Black"},
	"th.delta_white": {DE: "Δ Weiß", EN: "Δ White"},

	"common.no_players": {DE: "Noch keine Spieler.", EN: "No players yet."},
	"common.no_games":   {DE: "Noch keine Partien.", EN: "No games yet."},

	// ---- Index / start page ---------------------------------------------
	"title.start":          {DE: "Start", EN: "Home"},
	"index.signed_in_as":   {DE: "Angemeldet als %s.", EN: "Signed in as %s."},
	"index.your_groups":    {DE: "Deine Gruppen", EN: "Your groups"},
	"index.no_groups_html": {DE: `Du administrierst noch keine Gruppe. Lege eine über den MCP-Server an (Tool <code>create_group</code>).`, EN: `You don't administer any group yet. Create one through the MCP server (tool <code>create_group</code>).`},
	"index.read_handbook":  {DE: "📖 Handbuch lesen", EN: "📖 Read the manual"},
	"index.mcp_heading":    {DE: "MCP-Server", EN: "MCP server"},
	"index.mcp_desc":       {DE: "Verwaltungstool für Claude.ai. URL:", EN: "Management tool for Claude.ai. URL:"},
	"index.mcp_auth":       {DE: "Auth: OAuth/OIDC gegen id.levinkeller.de — Claude.ai entdeckt den Auth-Server selbst, beim ersten Verbinden öffnet sich ein Browser-Fenster.", EN: "Auth: OAuth/OIDC against id.levinkeller.de — Claude.ai discovers the auth server itself; a browser window opens the first time you connect."},

	// ---- Dashboard ------------------------------------------------------
	"dashboard.total_games": {DE: "Partien gesamt", EN: "Total games"},
	"dashboard.ranking":     {DE: "Rangliste", EN: "Ranking"},
	"dashboard.recent":      {DE: "Letzte Partien", EN: "Recent games"},
	"nav.players":           {DE: "Spieler", EN: "Players"},
	"title.players":         {DE: "Spieler", EN: "Players"},
	"players.heading":       {DE: "Spieler", EN: "Players"},
	"players.empty_html":    {DE: `Noch keine Spieler. Über die <a href="%s">Spielen-Seite</a> oder via MCP anlegen.`, EN: `No players yet. Add them via the <a href="%s">play page</a> or through MCP.`},

	// ---- Admins ---------------------------------------------------------
	"nav.admins":          {DE: "Admins", EN: "Admins"},
	"admin.mcp_note_html": {DE: `Admin-Verwaltung läuft über den MCP-Server: <code>add_admin</code> bzw. <code>remove_admin</code> mit <code>group: "%s"</code> und der E-Mail des Users.`, EN: `Admin management runs through the MCP server: <code>add_admin</code> or <code>remove_admin</code> with <code>group: "%s"</code> and the user's email.`},

	// ---- Player profile -------------------------------------------------
	"player.ratings":       {DE: "Bewertungen", EN: "Ratings"},
	"player.label_overall": {DE: "Gesamt", EN: "Overall"},
	"player.no_game":       {DE: "— noch keine Partie", EN: "— no games yet"},
	"player.ratings_note":  {DE: "„Gesamt“ ist die OGS-Bewertung über alle Partien; darunter je eine eigene Glicko-2-Bewertung pro Brettgröße. Das ± ist die Unsicherheit in Rangstufen.", EN: "“Overall” is the OGS rating across all games; below it, a separate Glicko-2 rating per board size. The ± is the uncertainty in rank steps."},
	"player.games":         {DE: "Partien", EN: "Games"},

	// ---- Play landing ---------------------------------------------------
	"play.hello":        {DE: "Hallo %s!", EN: "Hello %s!"},
	"play.what_to_do":   {DE: "Was möchtest du machen?", EN: "What would you like to do?"},
	"play.steps":        {DE: "%d Schritte", EN: "%d steps"},
	"play.calc_title":   {DE: "Vorgabe berechnen", EN: "Calculate handicap"},
	"play.calc_desc":    {DE: "Wer spielt gegen wen? Auf welchem Brett? Wir sagen die Vorgabe.", EN: "Who plays whom? On which board? We'll tell you the handicap."},
	"play.record_title": {DE: "Spiel eintragen", EN: "Record a game"},
	"play.record_desc":  {DE: "Spieler, Brett, Vorgabe und wer gewonnen hat — fertig.", EN: "Players, board, handicap and who won — done."},
	"play.played_today": {DE: "Heute schon gespielt", EN: "Played today"},
	"play.black_stone":  {DE: "⚫ Schwarz", EN: "⚫ Black"},
	"play.white_stone":  {DE: "⚪ Weiß", EN: "⚪ White"},

	// ---- Pick player / board wizard -------------------------------------
	"pick.no_players_html": {DE: "Keine Spieler in der Gruppe.<br><strong>Sag deinem Trainer Bescheid</strong> — neue Spieler werden über den MCP-Server angelegt.", EN: "No players in this group.<br><strong>Let your coach know</strong> — new players are added through the MCP server."},
	"pick.tap_name":        {DE: "Tippe auf einen Namen", EN: "Tap a name"},
	"board.fast":           {DE: "schnelle Partie", EN: "quick game"},
	"board.medium":         {DE: "mittel", EN: "medium"},
	"board.full":           {DE: "volles Brett", EN: "full board"},

	// Wizard step headlines (recommend + record flows).
	"wiz.rec.p1":       {DE: "Wer ist der erste Spieler?", EN: "Who is the first player?"},
	"wiz.rec.p2":       {DE: "Und wer ist Spieler 2?", EN: "And who is player 2?"},
	"wiz.board":        {DE: "Wie groß ist das Brett?", EN: "How big is the board?"},
	"wiz.game.p1":      {DE: "Wer hat gespielt? Spieler 1.", EN: "Who played? Player 1."},
	"wiz.game.p2":      {DE: "Und wer noch?", EN: "And who else?"},
	"wiz.game.board":   {DE: "Auf welchem Brett?", EN: "On which board?"},
	"wiz.game.who_won": {DE: "Wer hat gewonnen?", EN: "Who won?"},

	// Wizard page titles.
	"title.who_plays":  {DE: "Wer spielt?", EN: "Who plays?"},
	"title.and_second": {DE: "Und der zweite?", EN: "And the second?"},
	"title.player2":    {DE: "Spieler 2?", EN: "Player 2?"},
	"title.board_size": {DE: "Brettgröße", EN: "Board size"},
	"title.handicap":   {DE: "Vorgabe", EN: "Handicap"},
	"title.record":     {DE: "Spiel eintragen", EN: "Record a game"},
	"title.result":     {DE: "Ergebnis", EN: "Result"},
	"title.confirm":    {DE: "Bestätigen", EN: "Confirm"},
	"title.recorded":   {DE: "Eingetragen!", EN: "Recorded!"},
	"title.play":       {DE: "Spielen — %s", EN: "Play — %s"},
	"title.admins":     {DE: "%s — Admins", EN: "%s — Admins"},
	"title.docs":       {DE: "%s — Handbuch", EN: "%s — Manual"},

	// ---- Recommend result -----------------------------------------------
	"result.here_is":            {DE: "Hier ist die Vorgabe", EN: "Here's the handicap"},
	"result.on_board":           {DE: "Auf %s-Brett", EN: "On a %s board"},
	"result.black_plays":        {DE: "Schwarz spielt", EN: "Black plays"},
	"result.white_plays":        {DE: "Weiß spielt", EN: "White plays"},
	"result.even_label":         {DE: "Ebenes Spiel", EN: "Even game"},
	"result.no_handicap":        {DE: "Keine Vorgabe", EN: "No handicap"},
	"result.minus_points":       {DE: " — Schwarz bekommt am Ende Punkte abgezogen", EN: " — Black has points deducted at the end"},
	"result.handicap_for_black": {DE: "Vorgabe für Schwarz", EN: "Handicap for Black"},
	"result.stones":             {DE: "Steine", EN: "stones"},
	"result.record_result":      {DE: "Ergebnis eintragen", EN: "Record result"},

	// komi helper: positive komi vs. reverse komi.
	"komi.normal":  {DE: "Komi %.1f", EN: "Komi %.1f"},
	"komi.reverse": {DE: "Rückkomi %.1f", EN: "Reverse komi %.1f"},

	// ---- Record finish --------------------------------------------------
	"finish.swap":       {DE: "⇄ Farben tauschen", EN: "⇄ Swap colours"},
	"finish.as_played":  {DE: "So gespielt", EN: "As played"},
	"finish.komi_hint":  {DE: "Komi darf negativ sein (Rückkomi) — vor allem auf 9×9 sinnvoll. Es endet immer auf ,5, damit es kein Unentschieden geben kann.", EN: "Komi may be negative (reverse komi) — handy on 9×9 especially. It always ends in .5 so a game can't be a draw."},
	"finish.black_wins": {DE: "Schwarz gewinnt", EN: "Black wins"},
	"finish.white_wins": {DE: "Weiß gewinnt", EN: "White wins"},

	// ---- Confirm --------------------------------------------------------
	"confirm.title": {DE: "Wirklich eintragen?", EN: "Record this?"},
	"confirm.sub":   {DE: "Schau nochmal kurz drauf.", EN: "Take one more look."},
	"confirm.yes":   {DE: "Ja, eintragen", EN: "Yes, record it"},

	// ---- Done -----------------------------------------------------------
	"done.title":      {DE: "✅ Spiel eingetragen!", EN: "✅ Game recorded!"},
	"done.x_won":      {DE: "%s hat gewonnen", EN: "%s won"},
	"done.points":     {DE: "Punkte", EN: "Points"},
	"done.promoted":   {DE: "🏅 Aufgestiegen auf", EN: "🏅 Promoted to"},
	"done.demoted":    {DE: "Abgestiegen auf", EN: "Demoted to"},
	"done.rank":       {DE: "Rang:", EN: "Rank:"},
	"done.same":       {DE: "(gleich)", EN: "(same)"},
	"done.to_ranking": {DE: "🏆 Zur Rangliste", EN: "🏆 To the ranking"},
	"done.another":    {DE: "Noch ein Spiel eintragen", EN: "Record another game"},
	"nav.recorded":    {DE: "Eingetragen", EN: "Recorded"},

	// ---- Docs -----------------------------------------------------------
	"docs.heading": {DE: "Handbuch", EN: "Manual"},

	// ---- Settings -------------------------------------------------------
	"settings.title":         {DE: "Einstellungen", EN: "Settings"},
	"settings.language":      {DE: "Sprache", EN: "Language"},
	"settings.language_desc": {DE: "Wähle die Sprache der Oberfläche.", EN: "Choose the interface language."},
	"settings.saved":         {DE: "Gespeichert.", EN: "Saved."},
}
