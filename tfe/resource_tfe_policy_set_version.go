package tfe

import (
	"fmt"
	"log"

	tfe "github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceTFEPolicySetVersion() *schema.Resource {
	return &schema.Resource{
		Create: resourceTFEPolicySetVersionCreate,
		Read:   resourceTFEPolicySetVersionRead,
		Delete: resourceTFEPolicySetVersionDelete,

		Schema: map[string]*schema.Schema{
			"policy_set_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"policies_path": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"policies_path_contents_checksum": {
				Type:     schema.TypeString,
				Computed: true,
			},

			// This is really a computed property. However, marking it as "optional"
			// allows us to use ForceNew to tell Terraform to recreate the policy set
			// version if the contents of the policies source directory has changed.
			"policies_path_contents_changed": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},

			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"error_message": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceTFEPolicySetVersionRead(d *schema.ResourceData, meta interface{}) error {
	tfeClient := meta.(*tfe.Client)

	log.Printf("[DEBUG] Read policy set version: %s", d.Id())
	policySetVersion, err := tfeClient.PolicySetVersions.Read(ctx, d.Id())
	if err != nil {
		if err == tfe.ErrResourceNotFound {
			log.Printf("[DEBUG] Policy set version %s does not exist", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading policy set version %s: %v", d.Id(), err)
	}

	d.SetId(policySetVersion.ID)
	d.Set("status", policySetVersion.Status)
	d.Set("error_message", policySetVersion.ErrorMessage)

	log.Printf("[DEBUG] Compute checksum for policy set files")

	policiesPath := d.Get("policies_path").(string)
	currentChecksum := d.Get("policies_path_contents_checksum")

	newChecksum, err := hashPolicies(policiesPath)
	if err != nil {
		return fmt.Errorf("Error generating the checksum for the source path files: %v", err)
	}

	d.Set("policies_path_contents_changed", currentChecksum != newChecksum)
	d.Set("policies_path_contents_checksum", newChecksum)

	return nil
}

func resourceTFEPolicySetVersionCreate(d *schema.ResourceData, meta interface{}) error {
	tfeClient := meta.(*tfe.Client)

	policySetID := d.Get("policy_set_id").(string)
	policiesPath := d.Get("policies_path").(string)
	psv, err := tfeClient.PolicySetVersions.Create(ctx, policySetID)
	if err != nil {
		return fmt.Errorf("Error creating policy set version for policy set %s: %v", policySetID, err)
	}

	err = tfeClient.PolicySetVersions.Upload(ctx, *psv, policiesPath)
	if err != nil {
		return fmt.Errorf("Error uploading policies for policy set version %s: %v", psv.ID, err)
	}

	checksum, err := hashPolicies(policiesPath)
	if err != nil {
		return fmt.Errorf("Error generating the checksum for the source path files: %v", err)
	}

	d.Set("policies_path_contents_checksum", checksum)

	d.SetId(psv.ID)

	return resourceTFEPolicySetVersionRead(d, meta)
}

func resourceTFEPolicySetVersionDelete(d *schema.ResourceData, meta interface{}) error {
	// The delete operation is required for a ForceNew field.
	// ForceNew destroys and recreates the resource, according to the docs:
	// https://www.terraform.io/docs/extend/schemas/schema-behaviors.html#forcenew

	// This is left nil because there is no operation delete a Policy Set Version,
	// so this only returns nil.
	// https://www.terraform.io/docs/cloud/api/policy-sets.html#create-a-policy-set-version
	return nil
}
