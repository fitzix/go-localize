package models

type WeblateTranslationItem struct {
	LanguageCode string `json:"language_code"`
	FileUrl      string `json:"file_url"`
}

type WeblateTranslationResult struct {
	Results []WeblateTranslationItem `json:"results"`
}
