package main

import (
	"nano/framework"
	"net"
	"net/http"
	"context"
	"log"
	"net/http/httputil"
	"github.com/julienschmidt/httprouter"
	"fmt"
	"net/url"
	"io"
	"encoding/json"
	"path/filepath"
	"io/ioutil"
)

type FrontEndService struct {
	serviceListener net.Listener
	frontendServer  http.Server
	listenAddress   string
	backendHost     string
	backendURL      string
	reverseProxy    *httputil.ReverseProxy
	channelManager  *ChannelManager
	sessionManager  *SessionManager
	userManager     *UserManager
	framework.SimpleRunner
}

const (
	CurrentVersion = "0.7.1"
)

func CreateFrontEnd(configPath string) (service *FrontEndService, err error ) {
	var configFile = filepath.Join(configPath, ConfigFileName)
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return
	}
	var config FrontEndConfig
	err = json.Unmarshal(data, &config)
	if err != nil {
		return
	}

	service = &FrontEndService{}
	service.listenAddress = fmt.Sprintf("%s:%d", config.ListenAddress, config.ListenPort)
	service.serviceListener, err = net.Listen("tcp", service.listenAddress)
	if err != nil{
		return
	}
	service.backendHost = config.ServiceHost
	service.backendURL = fmt.Sprintf("http://%s:%d", config.ServiceHost, config.ServicePort)
	proxyUrl, err := url.Parse(service.backendURL)
	if err != nil{
		return
	}
	service.reverseProxy = httputil.NewSingleHostReverseProxy(proxyUrl)
	service.channelManager, _ = CreateChannelManager()
	var router = httprouter.New()
	service.registerHandler(router)
	router.ServeFiles("/css/*filepath", http.Dir("resource/css"))
	router.ServeFiles("/js/*filepath", http.Dir("resource/js"))
	router.NotFound = http.FileServer(http.Dir("resource"))

	service.frontendServer.Handler = router
	service.Initial(service)

	service.userManager, err = CreateUserManager(configPath)
	if err != nil{
		return
	}
	service.sessionManager, err = CreateSessionManager()
	if err != nil{
		return
	}
	return
}

func (service *FrontEndService)GetListenAddress() string{
	return service.listenAddress
}
func (service *FrontEndService)GetBackendURL() string{
	return service.backendURL
}

func (service *FrontEndService) GetVersion() string{
	return CurrentVersion
}

func (service *FrontEndService)Routine(){
	log.Printf("<frontend> %s started", CurrentVersion)
	go service.frontendServer.Serve(service.serviceListener)
	service.channelManager.Start()
	for !service.IsStopping(){
		select {
		case <- service.GetNotifyChannel():
			log.Println("<frontend> stopping server...")
			service.channelManager.Stop()
			service.SetStopping()
			//shutdown server
			ctx, _ := context.WithCancel(context.TODO())
			if err := service.frontendServer.Shutdown(ctx);err != nil{
				log.Printf("<frontsend> shutdown server fail: %s", err.Error())
			}else{
				log.Println("<frontend> server shutdown")
			}

		}
	}
	service.NotifyExit()
}

func (service *FrontEndService)registerHandler(router *httprouter.Router){
	const (
		GET    = iota
		POST
		PUT
		DELETE
	)

	var redirect = func(r *httprouter.Router, path string, method int) {
		switch method {
		case GET:
			r.GET(path, service.redirectToBackend)
		case POST:
			r.POST(path, service.redirectToBackend)
		case PUT:
			r.PUT(path, service.redirectToBackend)
		case DELETE:
			r.DELETE(path, service.redirectToBackend)
		default:
			log.Printf("<frontend> define redirect fail, invalid method %d", method)
		}
	}

	router.GET("/", service.defaultLandingPage)
	router.GET("/monitor_channels/:id", service.handleEstablishChannel)
	router.POST("/monitor_channels/", service.handleCreateChannel)

	//API
	redirect(router, "/instances/:id", GET)
	redirect(router, "/instances/:id", POST)
	redirect(router, "/instances/:id", DELETE)

	redirect(router, "/guests/:id", GET)
	redirect(router, "/guests/", POST)
	redirect(router, "/guests/:id", DELETE)

	redirect(router, "/guest_search/*filepath", GET)
	redirect(router, "/guest/:id/cores", PUT)
	redirect(router, "/guest/:id/memory", PUT)
	redirect(router, "/guest/:id/auth", PUT)
	redirect(router, "/guest/:id/auth", GET)
	redirect(router, "/guest/:id/disks/resize/:index", PUT)
	redirect(router, "/guest/:id/disks/shrink/:index", PUT)

	redirect(router, "/compute_zone_status/", GET)
	redirect(router, "/compute_pool_status/", GET)
	redirect(router, "/compute_pool_status/:pool", GET)
	redirect(router, "/compute_cell_status/:pool", GET)
	redirect(router, "/compute_cell_status/:pool/:cell", GET)
	redirect(router, "/instance_status/:pool", GET)
	redirect(router, "/instance_status/:pool/:cell", GET)

	redirect(router, "/compute_pools/", GET)
	redirect(router, "/compute_pools/:pool", GET)
	redirect(router, "/compute_pools/:pool", POST)
	redirect(router, "/compute_pools/:pool", PUT)
	redirect(router, "/compute_pools/:pool", DELETE)
	redirect(router, "/compute_pool_cells/", GET)
	redirect(router, "/compute_pool_cells/:pool", GET)
	redirect(router, "/compute_pool_cells/:pool/:cell", GET)
	redirect(router, "/compute_pool_cells/:pool/:cell", POST)
	redirect(router, "/compute_pool_cells/:pool/:cell", PUT)
	redirect(router, "/compute_pool_cells/:pool/:cell", DELETE)

	//address pool
	redirect(router, "/address_pools/", GET)
	redirect(router, "/address_pools/:pool", GET)
	redirect(router, "/address_pools/:pool", POST)
	redirect(router, "/address_pools/:pool", PUT)
	redirect(router, "/address_pools/:pool", DELETE)

	//address range
	redirect(router, "/address_pools/:pool/:type/ranges/", GET)
	redirect(router, "/address_pools/:pool/:type/ranges/:start", GET)
	redirect(router, "/address_pools/:pool/:type/ranges/:start", POST)
	redirect(router, "/address_pools/:pool/:type/ranges/:start", DELETE)

	//storage pools
	redirect(router, "/storage_pools/", GET)
	redirect(router, "/storage_pools/:pool", GET)
	redirect(router, "/storage_pools/:pool", POST)
	redirect(router, "/storage_pools/:pool", PUT)
	redirect(router, "/storage_pools/:pool", DELETE)

	redirect(router, "/media_images/", GET)
	redirect(router, "/media_images/", POST)
	redirect(router, "/media_images/:id", DELETE)
	redirect(router, "/media_image_files/:id", POST)

	redirect(router, "/disk_image_search/*filepath", GET)
	redirect(router, "/disk_images/:id", GET)
	redirect(router, "/disk_images/", POST)
	redirect(router, "/disk_images/:id", DELETE)
	redirect(router, "/disk_image_files/:id", GET)
	redirect(router, "/disk_image_files/:id", POST)

	redirect(router, "/instances/:id/media", POST)
	redirect(router, "/instances/:id/media", DELETE)

	redirect(router, "/instances/:id/snapshots/", GET)
	redirect(router, "/instances/:id/snapshots/", POST)
	redirect(router, "/instances/:id/snapshots/", PUT)
	redirect(router, "/instances/:id/snapshots/:name", GET)
	redirect(router, "/instances/:id/snapshots/:name", DELETE)

	//migrations
	redirect(router, "/migrations/", GET)
	redirect(router, "/migrations/:id", GET)
	redirect(router, "/migrations/", POST)
	
	//inner function
	
	//user roles
	router.GET("/roles/", service.queryRoles)
	router.GET("/roles/:role", service.getRole)
	router.POST("/roles/:role", service.addRole)
	router.PUT("/roles/:role", service.modifyRole)
	router.DELETE("/roles/:role", service.removeRole)

	//user groups
	router.GET("/user_groups/", service.queryGroups)
	router.GET("/user_groups/:group", service.getGroup)
	router.POST("/user_groups/:group", service.addGroup)
	router.PUT("/user_groups/:group", service.modifyGroup)
	router.DELETE("/user_groups/:group", service.removeGroup)

	router.GET("/user_groups/:group/members/", service.queryGroupMembers)
	router.POST("/user_groups/:group/members/:user", service.addGroupMember)
	router.DELETE("/user_groups:group/members/:user", service.removeGroupMember)

	router.GET("/user_groups/:group/roles/", service.queryGroupRoles)
	router.POST("/user_groups/:group/roles/:role", service.addGroupRole)
	router.DELETE("/user_groups:group/roles/:role", service.removeGroupRole)

	//users
	router.GET("/users/", service.queryUsers)
	router.GET("/users/:user", service.getUser)
	router.POST("/users/:user", service.createUser)
	router.PUT("/users/:user", service.modifyUser)
	router.DELETE("/users/:user", service.deleteUser)

	router.PUT("/users/:user/password/", service.modifyUserPassword)

	//sessions
	router.GET("/sessions/", service.querySessions)
	router.GET("/sessions/:session", service.getSession)
	router.POST("/sessions/:session", service.createSession)
	router.PUT("/sessions/:session", service.updateSession)

	////logs
	//router.GET("/logs/", service.queryLogs)
	//router.POST("/logs/", service.addLog)
}

func (service *FrontEndService)defaultLandingPage(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	const(
		DefaultURL = "/dashboard.html"
	)
	http.Redirect(w, r, DefaultURL, http.StatusMovedPermanently)
}

func (service *FrontEndService)redirectToBackend(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	r.Host = service.backendHost
	service.reverseProxy.ServeHTTP(w, r)
}


type Response struct {
	ErrorCode int         `json:"error_code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
}

const (
	DefaultServerError = 500
)

func ResponseFail(code int, message string, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(Response{code, message, struct{}{}})
}

func ResponseOK(data interface{}, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(Response{0, "", data})
}

//user roles

func (service *FrontEndService) queryRoles(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var data = make([]string, 0)
	var respChan = make(chan UserResult, 1)
	service.userManager.QueryRoles(respChan)
	var result = <- respChan
	if result.Error != nil{
		var err = result.Error
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK(data, w)
}

func (service *FrontEndService) getRole(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var roleName = params.ByName("role")
	var respChan = make(chan UserResult, 1)
	service.userManager.GetRole(roleName, respChan)
	var result = <- respChan
	if result.Error != nil{
		ResponseFail(DefaultServerError, result.Error.Error(), w)
		return
	}
	type ResponsePayload struct {
		Menu []string `json:"menu,omitempty"`
	}
	var payload = ResponsePayload{Menu:result.Role.Menu}
	ResponseOK(payload, w)
}

func (service *FrontEndService) addRole(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var roleName = params.ByName("role")
	type RequestData struct {
		Menu []string `json:"menu,omitempty"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	var respChan = make(chan error, 1)
	service.userManager.AddRole(roleName, requestData.Menu, respChan)
	err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) modifyRole(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var roleName = params.ByName("role")
	type RequestData struct {
		Menu []string `json:"menu,omitempty"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	var respChan = make(chan error, 1)
	service.userManager.ModifyRole(roleName, requestData.Menu, respChan)
	err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) removeRole(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var roleName = params.ByName("role")
	var respChan = make(chan error, 1)
	service.userManager.RemoveRole(roleName, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}


//user groups

func (service *FrontEndService) queryGroups(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	type RespGroup struct {
		Name    string `json:"name"`
		Display string `json:"display"`
		Member  int    `json:"member"`
	}
	var payload = make([]RespGroup, 0)
	var respChan = make(chan UserResult, 1)
	service.userManager.QueryGroups(respChan)
	var result = <- respChan
	if result.Error != nil{
		var err = result.Error
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	for _, group := range result.GroupList{
		var memberCount = len(group.Members)
		payload = append(payload, RespGroup{group.Name, group.Display, memberCount})
	}
	ResponseOK(payload, w)
}

func (service *FrontEndService) getGroup(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var respChan = make(chan UserResult, 1)
	service.userManager.GetGroup(groupName, respChan)
	var result = <- respChan
	if result.Error != nil{
		ResponseFail(DefaultServerError, result.Error.Error(), w)
		return
	}
	type RespGroup struct {
		Name    string   `json:"name"`
		Display string   `json:"display"`
		Member  []string `json:"member,omitempty"`
	}
	var group = result.Group
	var payload = RespGroup{Name: group.Name, Display: group.Display}
	for memberName, _ := range group.Members{
		payload.Member = append(payload.Member, memberName)
	}
	ResponseOK(payload, w)
}

func (service *FrontEndService) addGroup(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	type RequestData struct {
		Display string `json:"display"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	var respChan = make(chan error, 1)
	service.userManager.AddGroup(groupName, requestData.Display, respChan)
	err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) modifyGroup(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	type RequestData struct {
		Display string `json:"display"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	var respChan = make(chan error, 1)
	service.userManager.ModifyGroup(groupName, requestData.Display, respChan)
	err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) removeGroup(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var respChan = make(chan error, 1)
	service.userManager.RemoveGroup(groupName, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) queryGroupMembers(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var respChan = make(chan UserResult, 1)
	service.userManager.QueryGroupMembers(groupName, respChan)
	var result = <- respChan
	if result.Error != nil{
		ResponseFail(DefaultServerError, result.Error.Error(), w)
		return
	}
	var payload = make([]string, 0)
	for _, member := range result.UserList{
		payload = append(payload, member.Name)
	}
	ResponseOK(payload, w)
}

func (service *FrontEndService) addGroupMember(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var userName = params.ByName("user")
	var respChan = make(chan error, 1)
	service.userManager.AddGroupMember(groupName, userName, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) removeGroupMember(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var userName = params.ByName("user")
	var respChan = make(chan error, 1)
	service.userManager.RemoveGroupMember(groupName, userName, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) queryGroupRoles(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var respChan = make(chan UserResult, 1)
	service.userManager.QueryGroupRoles(groupName, respChan)
	var result = <- respChan
	if result.Error != nil{
		ResponseFail(DefaultServerError, result.Error.Error(), w)
		return
	}
	var payload = make([]string, 0)
	for _, role := range result.RoleList{
		payload = append(payload, role.Name)
	}
	ResponseOK(payload, w)
}

func (service *FrontEndService) addGroupRole(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var roleName = params.ByName("role")
	var respChan = make(chan error, 1)
	service.userManager.AddGroupRole(groupName, roleName, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) removeGroupRole(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var groupName = params.ByName("group")
	var roleName = params.ByName("role")
	var respChan = make(chan error, 1)
	service.userManager.RemoveGroupRole(groupName, roleName, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

//users

func (service *FrontEndService) queryUsers(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var payload = make([]string, 0)
	var respChan = make(chan UserResult, 1)
	service.userManager.QueryUsers(respChan)
	var result = <- respChan
	if result.Error != nil{
		var err = result.Error
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	for _, user := range result.UserList{
		payload = append(payload, user.Name)
	}
	ResponseOK(payload, w)
}

func (service *FrontEndService) getUser(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var userName = params.ByName("user")
	var respChan = make(chan UserResult, 1)
	service.userManager.GetUser(userName, respChan)
	var result = <- respChan
	if result.Error != nil{
		ResponseFail(DefaultServerError, result.Error.Error(), w)
		return
	}
	type RespUser struct {
		Name           string `json:"name"`
		Nick           string `json:"nick,omitempty"`
		Mail           string `json:"mail,omitempty"`
	}
	var user = result.User
	var payload = RespUser{Name: user.Name, Nick: user.Nick, Mail:user.Mail}
	ResponseOK(payload, w)
}

func (service *FrontEndService) createUser(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var userName = params.ByName("user")
	type RequestData struct {
		Nick     string `json:"nick,omitempty"`
		Mail     string `json:"mail,omitempty"`
		Password string `json:"password"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	var respChan = make(chan error, 1)
	service.userManager.CreateUser(userName, requestData.Nick, requestData.Mail, requestData.Password, respChan)
	err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) modifyUser(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var userName = params.ByName("user")
	type RequestData struct {
		Nick           string `json:"nick,omitempty"`
		Mail           string `json:"mail,omitempty"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	var respChan = make(chan error, 1)
	service.userManager.ModifyUser(userName, requestData.Nick, requestData.Mail, respChan)
	err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) deleteUser(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var userName = params.ByName("user")
	var respChan = make(chan error, 1)
	service.userManager.DeleteUser(userName, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

func (service *FrontEndService) modifyUserPassword(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var userName = params.ByName("user")
	type RequestData struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	var respChan = make(chan error, 1)
	service.userManager.ModifyUserPassword(userName, requestData.Old, requestData.New, respChan)
	err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

//sessions

func (service *FrontEndService) querySessions(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var payload = make([]string, 0)
	var respChan = make(chan SessionResult, 1)
	service.sessionManager.QuerySessions(respChan)
	var result = <- respChan
	if result.Error != nil{
		var err = result.Error
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	for _, session := range result.SessionList{
		payload = append(payload, session.ID)
	}
	ResponseOK(payload, w)
}

func (service *FrontEndService) getSession(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var sessionID = params.ByName("session")
	var respChan = make(chan SessionResult, 1)
	service.sessionManager.GetSession(sessionID, respChan)
	var result = <- respChan
	if result.Error != nil{
		ResponseFail(DefaultServerError, result.Error.Error(), w)
		return
	}
	type RespSession struct {
		User    string   `json:"user"`
		Menu    []string `json:"menu,omitempty"`
		Timeout int      `json:"timeout"`
	}
	var session = result.Session
	var payload = RespSession{session.User, session.Menu, session.Timeout}
	ResponseOK(payload, w)
}

func (service *FrontEndService) createSession(w http.ResponseWriter, r *http.Request, params httprouter.Params){

	type RequestData struct {
		User     string `json:"user"`
		Password string `json:"password"`
		Nonce    string `json:"nonce"`
	}
	var requestData RequestData
	var decoder = json.NewDecoder(r.Body)
	var err error
	if err = decoder.Decode(&requestData);err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	{
		//verify
		var respChan = make(chan error, 1)
		service.userManager.VerifyUserPassword(requestData.User, requestData.Password, respChan)
		err = <- respChan
		if err != nil{
			ResponseFail(DefaultServerError, err.Error(), w)
			return
		}
	}
	var user LoginUser
	{
		var respChan = make(chan UserResult, 1)
		service.userManager.GetUser(requestData.User, respChan)
		var result = <- respChan
		if result.Error != nil{
			ResponseFail(DefaultServerError, result.Error.Error(), w)
			return
		}
		user = result.User
	}
	{
		//allocate
		var respChan = make(chan SessionResult, 1)
		service.sessionManager.AllocateSession(user.Name, requestData.Nonce, user.Menu, respChan)
		var result = <- respChan
		if result.Error != nil{
			ResponseFail(DefaultServerError, result.Error.Error(), w)
			return
		}
		type RespSession struct {
			Session string `json:"session"`
		}
		var payload = RespSession{result.Session.ID}
		ResponseOK(payload, w)
	}
}

func (service *FrontEndService) updateSession(w http.ResponseWriter, r *http.Request, params httprouter.Params){
	var sessionID = params.ByName("session")
	var respChan = make(chan error, 1)
	service.sessionManager.UpdateSession(sessionID, respChan)
	var err = <- respChan
	if err != nil{
		ResponseFail(DefaultServerError, err.Error(), w)
		return
	}
	ResponseOK("", w)
}

//logs
func (service *FrontEndService) queryLogs(w http.ResponseWriter, r *http.Request, params httprouter.Params){

}

func (service *FrontEndService) addLog(w http.ResponseWriter, r *http.Request, params httprouter.Params){

}