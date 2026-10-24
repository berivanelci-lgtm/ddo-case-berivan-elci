// Package user, kullanıcı ve rol tanımlarını ile kullanıcı kayıtlarını yönetir.
package user

// Role, bir kullanıcının sistemdeki görevini belirtir.
type Role string

// Sistemdeki roller. Değerler veritabanındaki users_role_check kısıtıyla aynıdır.
const (
	RoleOperator        Role = "operator"
	RoleHatSefi         Role = "hat_sefi"
	RoleKaliteMuhendisi Role = "kalite_muhendisi"
	RoleUretimPlanlama  Role = "uretim_planlama"
	RoleBakim           Role = "bakim"
	RoleFabrikaMuduru   Role = "fabrika_muduru"
)

// AllRoles, sistemdeki tüm rolleri döner.
func AllRoles() []Role {
	return []Role{
		RoleOperator,
		RoleHatSefi,
		RoleKaliteMuhendisi,
		RoleUretimPlanlama,
		RoleBakim,
		RoleFabrikaMuduru,
	}
}

// Valid, rolün tanımlı rollerden biri olup olmadığını söyler.
func (r Role) Valid() bool {
	for _, known := range AllRoles() {
		if r == known {
			return true
		}
	}
	return false
}
