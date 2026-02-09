package docx

import "encoding/xml"

// Relationships represents the relationships file (word/_rels/document.xml.rels)
type Relationships struct {
	XMLName       xml.Name       `xml:"Relationships"`
	Relationships []Relationship `xml:"Relationship"`
}

// Relationship defines a single relationship
type Relationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"` // External for external links
}

// Relationship type constants
const (
	RelTypeHyperlink = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	RelTypeImage     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

// GetRelationship returns a relationship by ID
func (r *Relationships) GetRelationship(id string) *Relationship {
	for i := range r.Relationships {
		if r.Relationships[i].ID == id {
			return &r.Relationships[i]
		}
	}
	return nil
}

// GetTarget returns the target URL/path for a relationship ID
func (r *Relationships) GetTarget(id string) string {
	rel := r.GetRelationship(id)
	if rel == nil {
		return ""
	}
	return rel.Target
}

// IsHyperlink checks if a relationship ID points to a hyperlink
func (r *Relationships) IsHyperlink(id string) bool {
	rel := r.GetRelationship(id)
	if rel == nil {
		return false
	}
	return rel.Type == RelTypeHyperlink
}

// IsImage checks if a relationship ID points to an image
func (r *Relationships) IsImage(id string) bool {
	rel := r.GetRelationship(id)
	if rel == nil {
		return false
	}
	return rel.Type == RelTypeImage
}

// IsExternal checks if a relationship points to an external resource
func (r *Relationships) IsExternal(id string) bool {
	rel := r.GetRelationship(id)
	if rel == nil {
		return false
	}
	return rel.TargetMode == "External"
}

// GetHyperlinks returns all hyperlink relationships
func (r *Relationships) GetHyperlinks() []*Relationship {
	var result []*Relationship
	for i := range r.Relationships {
		if r.Relationships[i].Type == RelTypeHyperlink {
			result = append(result, &r.Relationships[i])
		}
	}
	return result
}

// GetImages returns all image relationships
func (r *Relationships) GetImages() []*Relationship {
	var result []*Relationship
	for i := range r.Relationships {
		if r.Relationships[i].Type == RelTypeImage {
			result = append(result, &r.Relationships[i])
		}
	}
	return result
}
