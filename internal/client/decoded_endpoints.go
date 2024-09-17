package client

import (
	"encoding/json"
	"fmt"
)

type Slt struct {
	SltId   int    `json:"SltId"`
	Content string `json:"Content"`
}

type Assignment struct {
	AssignmentContent       string   `json:"AssignmentContent"`
	AssignmentDecider       string   `json:"AssignmentDecider"`
	AllowedLearners         []string `json:"AllowedLearners"`
	PrerequisiteAssignments []string `json:"PrerequisiteAssignments"`
}

type DecodedDatum struct {
	ModuleCs   string     `json:"ModuleCs"`
	Slts       []Slt      `json:"Slts"`
	Assignment Assignment `json:"Assignment"`
}

type ModuleTokenResponse struct {
	ModuleToken  string       `json:"module_token"`
	DecodedDatum DecodedDatum `json:"decoded_datum"`
}

type CourseStateDatumResponse struct {
	CompletedAssignments []string `json:"CompletedAssignments"`
	CourseCs             string   `json:"CourseCs"`
	GlobalCs             string   `json:"GlobalCs"`
	CsdUserName          string   `json:"CsdUserName"`
}

type TokenInfo struct {
	LsCs           string   `json:"LsCs"`
	AssignmentList []string `json:"AssignmentList"`
	Minted         bool     `json:"Minted"`
}

type GlobalStateDatumResponse struct {
	UserCs     string      `json:"UserCs"`
	UserName   string      `json:"UserName"`
	TokenInfos []TokenInfo `json:"TokenInfos"`
	UserInfo   string      `json:"UserInfo"`
}

func GetDecodedAssignmentDatums(policy string) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/assignment-validator/decodedAssignmentDatumsByCourseNftPolicy")
	logResponse(resp, err)
}

func GetDecodedCourseStateDatum(policy string, alias string) (*CourseStateDatumResponse, error) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		SetQueryParam("alias", alias).
		Get("/course-state/decodedCourseStateDatumByCourseNftPolicyAndAlias")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode())
	}

	var result CourseStateDatumResponse

	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func GetDecodedGlobalStateDatum(alias string) (*GlobalStateDatumResponse, error) {
	resp, err := client.R().
		SetQueryParam("alias", alias).
		Get("/global-state/decodedGlobalStateDatumByAlias")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode())
	}

	var result GlobalStateDatumResponse

	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func GetDecodedModuleRefDatums(policy string) (*[]ModuleTokenResponse, error) {
	resp, err := client.R().
		SetQueryParam("policy", policy).
		Get("/module-ref/decodedModuleRefDatumsByCourseNftPolicy")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode())
	}

	var result []ModuleTokenResponse

	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
