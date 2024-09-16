package client

import (
	"github.com/go-resty/resty/v2"
)

var client *resty.Client

func init() {
	client = resty.New().
		SetBaseURL("https://dev.andamio.io/api").
		SetHeader("Content-Type", "application/json")
}

func GetAliasAvailability(alias string) {
	resp, err := client.R().
		SetQueryParam("alias", alias).
		Get("/aliasAvailability")
	logResponse(resp, err)
}

func GetAllGlobalStateUtxos() {
	resp, err := client.R().
		Get("/global-state/utxos")
	logResponse(resp, err)
}

func GetGlobalStateUtxo(alias string) {
	resp, err := client.R().
		SetQueryParam("alias", alias).
		Get("/global-state/utxoByAlias")
	logResponse(resp, err)
}

func GetDecodedGlobalStateDatum(alias string) {
	resp, err := client.R().
		SetQueryParam("alias", alias).
		Get("/global-state/decodedGlobalStateDatumByAlias")
	logResponse(resp, err)
}

func GetAllIndexValidatorUtxos() {
	resp, err := client.R().
		Get("/index-validator/utxos")
	logResponse(resp, err)
}

func GetInputUtxo(alias string) {
	resp, err := client.R().
		SetQueryParam("alias", alias).
		Get("/index-validator/utxoByNewAlias")
	logResponse(resp, err)
}

func GetAllInstanceValidatorUtxos() {
	resp, err := client.R().
		Get("/instance-validator/utxos")
	logResponse(resp, err)
}

func GetAllCourseInstanceUtxos() {
	resp, err := client.R().
		Get("/instance-validator/courseInstanceUtxos")
	logResponse(resp, err)
}

func GetCourseInstanceUtxo(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/instance-validator/courseInstanceUtxoByCourseNftPolicy")
	logResponse(resp, err)
}

func GetDecodedCourseInstanceDatum(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/instance-validator/decodedCourseInstanceDatumByCourseNftPolicy")
	logResponse(resp, err)
}

func GetLocalStatePolicyRefUtxo(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/instance-validator/localStatePolicyRefUtxoByCourseNftPolicy")
	logResponse(resp, err)
}

func GetLocalStateValidatorRefUtxo(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/instance-validator/localStateValildatorRefUtxoByCourseNftPolicy")
	logResponse(resp, err)
}

func GetModuleTokenPolicyRefUtxo(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/instance-validator/moduleTokenPolicyRefUtxoByCourseNftPolicy")
	logResponse(resp, err)
}

func GetModuleValidatorRefUtxo(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/instance-validator/moduleValidatorRefUtxoByCourseNftPolicy")
	logResponse(resp, err)
}

func GetAssignmentValidatorRefUtxo(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/instance-validator/assignmentValidatorRefUtxoByCourseNftPolicy")
	logResponse(resp, err)
}

func GetCourseStateAddress(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/course-state/courseStateAddressByCourseNftPolicy")
	logResponse(resp, err)
}

func GetCourseStateUtxos(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/course-state/courseStateUtxosByCourseNftPolicy")
	logResponse(resp, err)
}

func GetCourseStateUtxo(policy string, alias string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		SetQueryParam("alias", alias).
		Get("/course-state/courseStateUtxoByCourseNftPolicyAndAlias")
	logResponse(resp, err)
}

func GetDecodedCourseStateDatum(policy string, alias string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		SetQueryParam("alias", alias).
		Get("/course-state/decodedCourseStateDatumByCourseNftPolicyAndAlias")
	logResponse(resp, err)
}

func GetAssignmentValidatorAddresses(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/assignment-validator/assignmentValidatorAddressesByCourseNftPolicy")
	logResponse(resp, err)
}

func GetAssignmentValidatorUtxos(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/assignment-validator/assignmentValidatorUtxosByCourseNftPolicy")
	logResponse(resp, err)
}

func GetDecodedAssignmentDatums(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/assignment-validator/decodedAssignmentDatumsByCourseNftPolicy")
	logResponse(resp, err)
}

func GetAssignmentValidatorUtxo(policy string, alias string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		SetQueryParam("alias", alias).
		Get("/assignment-validator/assignmentValidatorUtxoByCourseNftPolicyAndAlias")
	logResponse(resp, err)
}

func GetDecodedAssignmentValidatorUtxoDatum(policy string, alias string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		SetQueryParam("alias", alias).
		Get("/assignment-validator/decodedAssignmentValidatorUtxoByCourseNftPolicyAndAlias")
	logResponse(resp, err)
}

func GetModuleRefValidatorAddress(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/module-ref/moduleRefValidatorAddressByCourseNftPolicy")
	logResponse(resp, err)
}

func GetModuleRefValidatorUtxos(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/module-ref/moduleRefValidatorUtxosByCourseNftPolicy")
	logResponse(resp, err)
}

func GetDecodedModuleRefDatums(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/module-ref/decodedModuleRefDatumsByCourseNftPolicy")
	logResponse(resp, err)
}

func GetModuleRefValidatorUtxo(policy string, token_name string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		SetQueryParam("token_name", token_name).
		Get("/module-ref/moduleRefValidatorUtxoByCourseNftPolicyAndTokenName")
	logResponse(resp, err)
}

func GetAllCourseGovernanceValidatorUtxos() {
	resp, err := client.R().
		Get("/course-governance-validator/utxos")
	logResponse(resp, err)
}

func GetCourseGovernanceValidatorUtxo(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/course-governance-validator/utxoByCourseNftPolicy")
	logResponse(resp, err)
}

func GetAllDecodedCourseGovDatums() {
	resp, err := client.R().
		Get("/course-governance-validator/decodedCourseGovDatums")
	logResponse(resp, err)
}

func GetCoursePolicies(alias string) {
	resp, err := client.R().
		SetQueryParam("alias", alias).
		Get("/course-governance-validator/creatorsCoursePoliciesByAlias")
	logResponse(resp, err)
}

func GetMintAccessToken(userAddress string, alias string, userInfo string) {
	resp, err := client.R().
		SetQueryParam("userAddress", userAddress).
		SetQueryParam("alias", alias).
		SetQueryParam("userInfo", userInfo).
		Get("/txs/mintAccessToken")
	logResponse(resp, err)
}

func GetMintLocalState(userAccessToken string, policy string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("policy", policy).
		Get("/txs/student-actions/mintLocalState")
	logResponse(resp, err)
}

func GetCommitToAssignment(userAccessToken string, policy string, assignmentCode string, assignmentInfo string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("policy", policy).
		SetQueryParam("assignmentCode", assignmentCode).
		SetQueryParam("assignmentInfo", assignmentInfo).
		Get("/txs/student-actions/commitToAssignment")
	logResponse(resp, err)
}

func GetUpdateAssignment(userAccessToken string, policy string, assignmentInfo string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("policy", policy).
		SetQueryParam("assignmentInfo", assignmentInfo).
		Get("/txs/student-actions/update-assignment")
	logResponse(resp, err)
}

func GetLeaveAssignment(userAccessToken string, policy string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("policy", policy).
		Get("/txs/student-actions/leave-assignment")
	logResponse(resp, err)
}

func GetBurnLocalState(userAccessToken string, policy string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("policy", policy).
		Get("/txs/student-actions/burnLocalState")
	logResponse(resp, err)
}

func GetMintModuleTokens(userAccessToken string, policy string, moduleInfos string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("policy", policy).
		SetQueryParam("moduleInfos", moduleInfos).
		Get("/txs/course-creator-actions/mintModuleTokens")
	logResponse(resp, err)
}

func GetAcceptAssignment(userAccessToken string, studentAlias string, policy string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("studentAlias", studentAlias).
		SetQueryParam("policy", policy).
		Get("/txs/course-creator-actions/acceptAssignment")
	logResponse(resp, err)
}

func GetDenyAssignment(userAccessToken string, studentAlias string, policy string) {
	resp, err := client.R().
		SetQueryParam("userAccessToken", userAccessToken).
		SetQueryParam("studentAlias", studentAlias).
		SetQueryParam("policy", policy).
		Get("/txs/course-creator-actions/denyAssignment")
	logResponse(resp, err)
}
